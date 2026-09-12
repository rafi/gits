package bulk

import (
	"context"
	"fmt"
	"sync"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/types"
)

// task is one unit of work: a repository plus its owning project and a stable
// index into the flattened result slice.
type task struct {
	project domain.Project
	repo    domain.Repository
	idx     int
}

// node is a project in render order, holding the stable indices of its own
// repositories so results print under the right title.
type node struct {
	project  domain.Project
	taskIdxs []int
}

// collect runs the body over every repository in the tree with a single
// bounded worker pool: every worker stays busy until the queue drains (no
// batch barrier), results land in indexed slots (no shared append) and come
// back grouped per project in stable tree order. Live progress is Diagnostic
// Output. The returned error is non-nil when the run was cut short, and names
// how many repositories were never processed — so an interrupted run fails
// loudly instead of reporting partial success.
func (c Command[T]) collect(
	ctx context.Context,
	project domain.Project,
	widths titleWidths,
	deps types.RuntimeCLI,
) ([]Group[T], error) {
	nodes, tasks := flatten(project)
	results := make([]*Result[T], len(tasks))

	workers := max(deps.Settings.WorkerCount, 1) // a pool of none processes nothing

	progress := newReporter(deps.Err)
	progress.Start(c.Verb, len(tasks))

	var wg sync.WaitGroup
	taskCh := make(chan task)

	for range workers {
		wg.Go(func() {
			for t := range taskCh {
				tracker := progress.RepoStart(t.repo.GetName())
				res := c.one(ctx, Repo{
					Repository: t.repo,
					Project:    t.project,
					Title:      paddedRepoTitle(t.repo, t.project, widths.For(t.project), deps),
				}, deps)
				results[t.idx] = &res
				if isFailure(res.Err) {
					// The tracker owns the run-wide failed-count.
					tracker.MarkErrored()
				} else {
					tracker.MarkDone()
				}
				progress.Done()
			}
		})
	}

	// Feed tasks, stopping early if the context is canceled so queued
	// repositories are never started.
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

	// Stop and drain the progress renderer before the renderer flushes
	// results, so Diagnostic Output never races Result Output.
	progress.Stop()

	groups := make([]Group[T], len(nodes))
	for gi, n := range nodes {
		g := Group[T]{Project: n.project}
		for _, idx := range n.taskIdxs {
			g.Results = append(g.Results, results[idx])
		}
		groups[gi] = g
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
	return groups, interrupted
}

// flatten walks the project tree depth-first (a project's repositories before
// its sub-projects) into render nodes and a parallel flat task list with
// stable indices.
//
// This order is a contract, not an implementation detail: status's JSON
// renderer inverts it positionally to rebuild the tree, consuming one subtree
// per sub-project. Reordering the traversal here reparents that output.
func flatten(root domain.Project) ([]node, []task) {
	var (
		nodes []node
		tasks []task
	)
	var visit func(p domain.Project)
	visit = func(p domain.Project) {
		n := node{project: p}
		for _, r := range p.Repos {
			idx := len(tasks)
			tasks = append(tasks, task{project: p, repo: r, idx: idx})
			n.taskIdxs = append(n.taskIdxs, idx)
		}
		nodes = append(nodes, n)
		for _, sub := range p.SubProjects {
			visit(sub)
		}
	}
	visit(root)
	return nodes, tasks
}
