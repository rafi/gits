// Package walk runs a bulk git operation across every repository in a project
// tree using a single bounded worker pool. It keeps every worker busy until the
// queue drains (no batch barrier), aggregates results into indexed slots (no
// shared append), renders them in stable tree order under each project header,
// shows live progress on stderr, and cancels cleanly via context.
package walk

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"

	"charm.land/lipgloss/v2"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cli"
	"github.com/rafi/gits/internal/types"
)

// RepoFunc runs one repo's work and returns a renderable result. It must be
// safe to call concurrently and must not write to stdout itself.
type RepoFunc func(
	ctx context.Context,
	project domain.Project,
	repo domain.Repository,
	deps types.RuntimeCLI,
) RepoResult

// RepoResult is the outcome of a single repo's work.
type RepoResult struct {
	Line string // fully rendered output for this repo (may be multi-line)
	Err  error  // nil, a real error, or a *types.Warning
}

// task is one unit of work: a repo plus its owning project and a stable index
// into the flattened result slice.
type task struct {
	project domain.Project
	repo    domain.Repository
	idx     int
}

// group is a project node in render order, holding the stable indices of its
// own repos so results print under the right header.
type group struct {
	project  domain.Project
	taskIdxs []int
}

// Walk runs fn over every repo in project (and its sub-projects), rendering
// progress to stderr and ordered results to stdout. It returns every result's
// error in stable tree order; callers pass them to cli.RenderErrors.
func Walk(
	ctx context.Context,
	project domain.Project,
	deps types.RuntimeCLI,
	verb string,
	fn RepoFunc,
) []error {
	return walkTo(ctx, project, deps, verb, fn, os.Stdout, os.Stderr)
}

// walkTo is Walk with injectable result/progress writers, for testing.
func walkTo(
	ctx context.Context,
	project domain.Project,
	deps types.RuntimeCLI,
	verb string,
	fn RepoFunc,
	resultsW io.Writer,
	progressW io.Writer,
) []error {
	return walkReport(ctx, project, deps, verb, fn, resultsW, NewReporter(progressW))
}

// walkReport is walkTo with an injectable reporter, for testing the per-repo
// tracker lifecycle with a fake.
func walkReport(
	ctx context.Context,
	project domain.Project,
	deps types.RuntimeCLI,
	verb string,
	fn RepoFunc,
	resultsW io.Writer,
	reporter Reporter,
) []error {
	groups, tasks := flatten(project)
	results := make([]*RepoResult, len(tasks))

	workers := max(deps.Settings.WorkerCount, 1) // AC-2: clamp to at least one worker

	reporter.Start(verb, len(tasks), workers)

	var (
		wg       sync.WaitGroup
		mu       sync.Mutex // guards errCount; results slots are index-disjoint
		errCount int
	)
	taskCh := make(chan task)

	for range workers {
		wg.Go(func() {
			for t := range taskCh {
				tracker := reporter.RepoStart(t.repo.GetName())
				res := fn(ctx, t.project, t.repo, deps)
				results[t.idx] = &res
				if isFailure(res.Err) {
					tracker.MarkErrored()
					// Fold the count into the reporter while holding mu so
					// counts are delivered in monotonic order; a late worker
					// must not settle the spinner on a stale, smaller value.
					mu.Lock()
					errCount++
					reporter.SetErrors(errCount)
					mu.Unlock()
				} else {
					tracker.MarkDone()
				}
				reporter.Done()
			}
		})
	}

	// Feed tasks, stopping early if the context is cancelled so queued repos
	// are never started (AC-9).
feed:
	for i := range tasks {
		select {
		case <-ctx.Done():
			break feed
		case taskCh <- tasks[i]:
		}
	}
	close(taskCh)
	wg.Wait()

	// Stop and drain the progress renderer before flushing results so stderr
	// progress never races stdout output (AC-4a).
	reporter.Stop()

	return render(resultsW, groups, results, deps)
}

// flatten walks the project tree depth-first (a project's repos before its
// sub-projects) into render groups and a parallel flat task list with stable
// indices.
func flatten(root domain.Project) ([]group, []task) {
	var (
		groups []group
		tasks  []task
	)
	var visit func(p domain.Project)
	visit = func(p domain.Project) {
		g := group{project: p}
		for _, r := range p.Repos {
			idx := len(tasks)
			tasks = append(tasks, task{project: p, repo: r, idx: idx})
			g.taskIdxs = append(g.taskIdxs, idx)
		}
		groups = append(groups, g)
		for _, sub := range p.SubProjects {
			visit(sub)
		}
	}
	visit(root)
	return groups, tasks
}

// render prints results in stable tree order grouped under each project header
// and returns the aggregated errors in the same order.
func render(
	w io.Writer,
	groups []group,
	results []*RepoResult,
	deps types.RuntimeCLI,
) []error {
	var errs []error
	for gi, g := range groups {
		if gi > 0 {
			fmt.Fprintln(w)
		}
		lipgloss.Fprintln(w, cli.ProjectTitleWithBullet(g.project, deps.Theme))
		for _, idx := range g.taskIdxs {
			res := results[idx]
			if res == nil {
				continue // not started (cancelled before dequeue)
			}
			lipgloss.Fprintln(w, cli.IndentMultiline(res.Line))
			if res.Err != nil {
				errs = append(errs, res.Err)
			}
		}
	}
	return errs
}

// isFailure reports whether err counts as a real failure (not a warning) for
// the live progress error count, mirroring cli.RenderErrors(_, true).
func isFailure(err error) bool {
	if err == nil {
		return false
	}
	var w *types.Warning
	if errors.As(err, &w) && w.Type == types.WarningType {
		return false
	}
	return true
}
