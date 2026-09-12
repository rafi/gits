package bulk

import (
	"context"
	"fmt"
	"sync"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/types"
)

// collect runs the body over every repository in the tree with a single
// bounded worker pool: every worker stays busy until the queue drains (no
// batch barrier), results land in indexed slots (no shared append) and come
// back in stable tree order. Live progress is Diagnostic Output. The
// interruption error is set when the run was cut short, and names how many
// repositories were never processed — so an interrupted run fails loudly
// instead of reporting partial success.
func (c Command[T]) collect(
	ctx context.Context,
	project domain.Project,
	deps types.RuntimeCLI,
) Results[T] {
	repos := flatten(project, deps.HomeDir)
	slots := make([]*Result[T], len(repos))

	workers := max(deps.Settings.WorkerCount, 1) // a pool of none processes nothing

	progress := newReporter(deps.Err)
	progress.Begin(c.Verb, len(repos))

	var wg sync.WaitGroup
	taskCh := make(chan int)

	for range workers {
		wg.Go(func() {
			for i := range taskCh {
				finish := progress.Start(repos[i].GetName())
				res := c.one(ctx, repos[i], deps)
				slots[i] = &res
				finish(isFailure(res.Err))
			}
		})
	}

	// Feed tasks, stopping early if the context is canceled so queued
	// repositories are never started.
feed:
	for i := range repos {
		select {
		case <-ctx.Done():
			break feed
		case taskCh <- i:
		}
	}
	close(taskCh)
	wg.Wait()

	// Stop and drain the progress renderer before the renderer flushes
	// results, so Diagnostic Output never races Result Output.
	progress.Stop()

	res := Results[T]{Project: project, Results: make([]Result[T], 0, len(slots))}
	skipped := 0
	for _, slot := range slots {
		if slot == nil {
			skipped++ // not started: canceled before dequeue
			continue
		}
		res.Results = append(res.Results, *slot)
	}
	if ctx.Err() != nil {
		res.Interrupted = fmt.Errorf(
			"interrupted: %d of %d repositories not processed", skipped, len(slots))
	}
	return res
}

// flatten walks the project tree depth-first — a project's repositories
// before its sub-projects — into the flat list the pool works through and
// the results come back in. Each repository carries its project's position in
// the walk as ProjectKey, so a renderer can tell two nodes apart even when
// they share a name.
func flatten(root domain.Project, homeDir string) []Repo {
	var repos []Repo
	var visit func(p domain.Project, key string)
	visit = func(p domain.Project, key string) {
		for _, r := range p.Repos {
			repos = append(repos, newRepo(r, p, key, homeDir))
		}
		for i, sub := range p.SubProjects {
			visit(sub, fmt.Sprintf("%s/%d", key, i))
		}
	}
	visit(root, "0")
	return repos
}
