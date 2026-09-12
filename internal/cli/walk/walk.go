// Package walk runs a bulk git operation across every repository in a project
// tree using a single bounded worker pool. It keeps every worker busy until the
// queue drains (no batch barrier), aggregates results into indexed slots (no
// shared append), renders them in stable tree order under each project header,
// shows live progress on stderr, and cancels cleanly via context.
package walk

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
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
	Line    string // fully rendered output for this repo (may be multi-line)
	Payload any    // structured result for callers rendering via Collect
	Err     error  // nil, a real error, or a *types.Warning
}

// LineResult applies err to line and wraps its rendering into a RepoResult —
// the shared epilogue of every cli.RepoLine-based RepoFunc.
func LineResult(line cli.RepoLine, err error) RepoResult {
	line.Err = err
	return RepoResult{Line: line.String(), Err: err}
}

// GroupResult is one project's collected results in stable tree order. A nil
// slot means the repo was never started (cancelled before dequeue).
type GroupResult struct {
	Project domain.Project
	Results []*RepoResult
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

// Collect runs fn over every repo like Walk, but returns the buffered results
// grouped per project instead of rendering lines, for callers that need the
// whole tree before formatting (e.g. status table column sizing). The error
// is non-nil when the run was interrupted before every repo was processed.
func Collect(
	ctx context.Context,
	project domain.Project,
	deps types.RuntimeCLI,
	verb string,
	fn RepoFunc,
) ([]GroupResult, error) {
	return collectReport(ctx, project, deps, verb, fn, NewReporter(os.Stderr))
}

// Single runs fn for one repository and renders its result line to stdout
// without a project header — the single-repo counterpart of Walk. The repo's
// error is returned as-is so warnings still downgrade the exit code at the
// root instead of being counted as failures.
func Single(
	ctx context.Context,
	project domain.Project,
	repo domain.Repository,
	deps types.RuntimeCLI,
	fn RepoFunc,
) error {
	return singleTo(ctx, project, repo, deps, fn, os.Stdout)
}

// singleTo is Single with an injectable writer, for testing.
func singleTo(
	ctx context.Context,
	project domain.Project,
	repo domain.Repository,
	deps types.RuntimeCLI,
	fn RepoFunc,
	w io.Writer,
) error {
	res := fn(ctx, project, repo, deps)
	lipgloss.Fprintln(w, cli.IndentMultiline(res.Line))
	return res.Err
}

// SingleCollect runs fn for one repository and returns the one-group shape
// Collect produces, plus the repo's error — the single-repo counterpart of
// Collect for callers that render grouped output themselves.
func SingleCollect(
	ctx context.Context,
	project domain.Project,
	repo domain.Repository,
	deps types.RuntimeCLI,
	fn RepoFunc,
) ([]GroupResult, error) {
	res := fn(ctx, project, repo, deps)
	groups := []GroupResult{
		{Project: project, Results: []*RepoResult{&res}},
	}
	return groups, res.Err
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
	groups, interrupted := collectReport(ctx, project, deps, verb, fn, reporter)
	return WithInterrupted(render(resultsW, groups, deps), interrupted)
}

// WithInterrupted appends the run-interrupted error from Collect (if any) to
// the rendered errors, so Collect-based callers report an interruption exactly
// like Walk does.
func WithInterrupted(errs []error, interrupted error) []error {
	if interrupted != nil {
		errs = append(errs, interrupted)
	}
	return errs
}

// collectReport is the shared worker-pool core: it runs fn over the flattened
// tree with live progress and returns every result grouped per project. On
// context cancellation the returned error names how many repos were never
// processed, so interrupted runs fail loudly instead of reporting partial
// success.
func collectReport(
	ctx context.Context,
	project domain.Project,
	deps types.RuntimeCLI,
	verb string,
	fn RepoFunc,
	reporter Reporter,
) ([]GroupResult, error) {
	groups, tasks := flatten(project)
	results := make([]*RepoResult, len(tasks))

	workers := max(deps.Settings.WorkerCount, 1) // AC-2: clamp to at least one worker

	reporter.Start(verb, len(tasks))

	var wg sync.WaitGroup
	taskCh := make(chan task)

	for range workers {
		wg.Go(func() {
			for t := range taskCh {
				tracker := reporter.RepoStart(t.repo.GetName())
				res := fn(ctx, t.project, t.repo, deps)
				results[t.idx] = &res
				if isFailure(res.Err) {
					// The tracker owns the run-wide failed-count.
					tracker.MarkErrored()
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

	grouped := make([]GroupResult, len(groups))
	for gi, g := range groups {
		gr := GroupResult{Project: g.project}
		for _, idx := range g.taskIdxs {
			gr.Results = append(gr.Results, results[idx])
		}
		grouped[gi] = gr
	}

	var interrupted error
	if ctx.Err() != nil {
		skipped := 0
		for _, res := range results {
			if res == nil {
				skipped++
			}
		}
		interrupted = fmt.Errorf(
			"interrupted: %d of %d repositories not processed", skipped, len(results))
	}
	return grouped, interrupted
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

// RenderGroups prints one block per project group and returns every non-nil
// result's error in stable tree order. It encodes the shared traversal
// contract once: nil result slots are skipped (cancelled before dequeue),
// errors are collected from every result — including ones the body chooses
// not to show — a blank line separates printed groups, and each shown group
// gets its project title (when withTitles) above the body. The body callback
// returns the group's rendered content ("" prints nothing under the title)
// and whether the group appears at all.
func RenderGroups(
	w io.Writer,
	groups []GroupResult,
	deps types.RuntimeCLI,
	withTitles bool,
	body func(GroupResult) (string, bool),
) []error {
	var errs []error
	printed := 0
	for _, g := range groups {
		for _, res := range g.Results {
			if res == nil {
				continue // not started (cancelled before dequeue)
			}
			if res.Err != nil {
				errs = append(errs, res.Err)
			}
		}
		content, show := body(g)
		if !show {
			continue
		}
		if printed > 0 {
			fmt.Fprintln(w)
		}
		if withTitles {
			lipgloss.Fprintln(w, cli.ProjectTitleWithBullet(g.Project, deps.Theme))
		}
		printed++
		if content != "" {
			lipgloss.Fprintln(w, content)
		}
	}
	return errs
}

// render prints results in stable tree order grouped under each project header
// and returns the aggregated errors in the same order.
func render(
	w io.Writer,
	groups []GroupResult,
	deps types.RuntimeCLI,
) []error {
	return RenderGroups(w, groups, deps, true, func(g GroupResult) (string, bool) {
		var lines []string
		for _, res := range g.Results {
			if res == nil {
				continue
			}
			lines = append(lines, cli.IndentMultiline(res.Line))
		}
		return strings.Join(lines, "\n"), true
	})
}

// isFailure reports whether err counts as a real failure (not a warning) for
// the live progress error count, mirroring cli.RenderErrors(_, true).
func isFailure(err error) bool {
	return err != nil && !types.IsWarning(err)
}
