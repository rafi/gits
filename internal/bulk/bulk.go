// Package bulk runs a Bulk Command: one operation applied to every repository
// in a project tree, reporting the outcome per repository.
//
// A command declares four things — a verb, the Repo States it acts on, an
// optional project skip and a per-repository body — and Run owns everything
// else: argument resolution and interactive selection, pruning, dispatch
// between the whole tree and a single repository, the Repo State guard, the
// bounded worker pool with live progress, stable tree ordering and
// interruption accounting. Run hands back the collected results; Lines is the
// stock renderer for them, and a command with its own output shape renders
// them itself and closes with Epilogue.
//
// Result Output and Diagnostic Output are read from the caller's dependencies;
// nothing here names a process stream.
package bulk

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cli"
	"github.com/rafi/gits/internal/types"
)

// Command is one Bulk Command. T is whatever its body produces for each
// repository: the rendered body text for the commands using Lines, a
// structured value for one rendering its own output.
type Command[T any] struct {
	// Verb labels the run in the live progress reporter ("pulling").
	Verb string
	// Accepts are the Repo States the body is willing to receive. A nil set
	// means the ok state: the permissive default's failure mode is a body
	// dereferencing a path that is not there, which is what the guard exists
	// to prevent. A repository failing the guard carries its state error as
	// its result and never reaches the body.
	Accepts []domain.RepoState
	// Skip drops a project and everything beneath it before the run. Unlike
	// the guard, a skipped project vanishes: never queued, never rendered,
	// and absent from the progress reporter's total. A nil Skip visits
	// everything.
	Skip func(domain.Project) bool
	// Body does one repository's work. It must be safe to call concurrently
	// and must write to no destination itself. The error it returns is kept
	// as it is: shown bare on the repository's own line, and wrapped with
	// the repository's name and path only in the error epilogue. A warning
	// built with types.NewWarning is a documented pass-over — it shows on
	// the line and does not fail the run.
	Body func(context.Context, Repo, types.RuntimeCLI) (T, error)
}

// Repo is the bundled per-repository argument a body receives: the repository,
// its owning project, and its display path.
type Repo struct {
	domain.Repository

	// Project is the repository's owning project — the resolved project
	// itself when a single repository was named.
	Project domain.Project
	// ProjectKey identifies that owning project within the run: results
	// carrying the same key came from the same node of the tree. Renderers
	// group on it rather than comparing project values, which are copied
	// freely as the tree is walked.
	ProjectKey string
	// Path is the repository's display path, relative to its project with ~
	// for the home directory. Lines pads it to the widest in the project.
	Path string
}

// Result is one repository's outcome. Value is the body's own, so no caller
// asserts a type at this seam; Err is exactly what the body or the state
// guard returned.
type Result[T any] struct {
	Repo  Repo
	Value T
	Err   error
}

// Results is everything a renderer receives: the tree the run visited, one
// result per repository that was started, in stable tree order, and the
// interruption error when the run was cut short. A repository the run never
// started — canceled before it was dequeued — has no result; Interrupted
// says how many there were.
type Results[T any] struct {
	// Project is the pruned tree the run visited. For a single named
	// repository it holds that repository alone, with no sub-projects. It is
	// the zero value when the named project was skipped.
	Project     domain.Project
	Results     []Result[T]
	Interrupted error
}

// Errors returns every result's error in tree order — a plain error wrapped
// with its repository's name and path, a warning as it is — followed by the
// interruption, if any. This is the list the error epilogue renders.
func (r Results[T]) Errors() []error {
	var errs []error
	for _, res := range r.Results {
		if res.Err == nil {
			continue
		}
		var wrapped *types.Warning
		if errors.As(res.Err, &wrapped) {
			errs = append(errs, res.Err)
			continue
		}
		errs = append(errs, cli.RepoError(res.Err, res.Repo.Repository))
	}
	if r.Interrupted != nil {
		errs = append(errs, r.Interrupted)
	}
	return errs
}

// Run resolves the arguments — prompting for a project or repository when they
// are missing — and executes the command over what they named. The error is
// argument resolution's; a repository's failure is in its result.
func (c Command[T]) Run(args []string, deps types.RuntimeCLI) (Results[T], error) {
	project, repo, err := cli.ParseArgs(args, true, deps)
	if err != nil {
		return Results[T]{}, err
	}

	// Pruning precedes the dispatch so a skipped project is skipped on both
	// paths: how the command was invoked must not override its configuration.
	project, kept := c.prune(project, deps.Err)
	if !kept {
		// The named project is skipped, so there is nothing to run. prune has
		// already said so as Diagnostic Output.
		return Results[T]{}, nil
	}

	if repo != nil {
		return c.single(deps.Ctx, project, *repo, deps), nil
	}
	return c.collect(deps.Ctx, project, deps), nil
}

// single runs the body for one named repository. The tree it reports is the
// project narrowed to that repository, so a renderer walks the same shape it
// would for a whole run. A repository whose project was pruned yields no
// result at all: it is not run, and nothing is rendered for it.
func (c Command[T]) single(
	ctx context.Context,
	project domain.Project,
	repo domain.Repository,
	deps types.RuntimeCLI,
) Results[T] {
	// Argument resolution found this repository in the tree, and pruning
	// shares every surviving project's Repos array — so the only way it is
	// missing now is that Skip dropped a project between it and the root,
	// which prune has already reported. Nothing to run, and nothing failed.
	if !inTree(project, repo) {
		return Results[T]{}
	}
	project.Repos = []domain.Repository{repo}
	project.SubProjects = nil
	res := c.one(ctx, newRepo(repo, project, "0", deps.HomeDir), deps)
	return Results[T]{Project: project, Results: []Result[T]{res}}
}

// newRepo bundles a repository with its project, the project's run-local key
// and its display path.
func newRepo(
	repo domain.Repository,
	project domain.Project,
	projectKey string,
	homeDir string,
) Repo {
	return Repo{
		Repository: repo,
		Project:    project,
		ProjectKey: projectKey,
		Path:       cli.RepoRelPath(project, repo, homeDir),
	}
}

// one applies the state guard and, when it passes, the body — the whole of
// what happens to a single repository, on either dispatch path.
func (c Command[T]) one(ctx context.Context, repo Repo, deps types.RuntimeCLI) Result[T] {
	res := Result[T]{Repo: repo}
	if !c.accepts(repo.State) {
		res.Err = cli.StateError(repo.Repository)
		return res
	}
	res.Value, res.Err = c.Body(ctx, repo, deps)
	return res
}

// accepts reports whether the body is willing to receive a repository in this
// state. A command that declares none accepts only `ok`.
func (c Command[T]) accepts(state domain.RepoState) bool {
	if len(c.Accepts) == 0 {
		return state == domain.RepoStateOK
	}
	return slices.Contains(c.Accepts, state)
}

// prune returns the tree the run visits and whether p itself survived, naming
// every skipped project as Diagnostic Output so a project that vanishes is
// distinguishable from an empty one. A skipped project is dropped whole — its
// repositories, its sub-projects and its own title — which is what makes it
// vanish rather than appear as an empty block. The original tree is left
// unmodified.
func (c Command[T]) prune(p domain.Project, w io.Writer) (domain.Project, bool) {
	if c.Skip == nil {
		return p, true
	}
	if c.Skip(p) {
		// Saying so once is the message: the whole subtree goes with it.
		fmt.Fprintf(w, "Skipping %s: excluded by configuration\n", p.Name)
		return p, false
	}
	subs := make([]domain.Project, 0, len(p.SubProjects))
	for _, sub := range p.SubProjects {
		if kept, ok := c.prune(sub, w); ok {
			subs = append(subs, kept)
		}
	}
	p.SubProjects = subs
	return p, true
}

// inTree reports whether repo survived pruning, identifying it by its Key,
// which is unique across a project tree.
func inTree(p domain.Project, repo domain.Repository) bool {
	for _, r := range p.Repos {
		if r.Key() == repo.Key() {
			return true
		}
	}
	for _, sub := range p.SubProjects {
		if inTree(sub, repo) {
			return true
		}
	}
	return false
}

// isFailure reports whether err counts as a real failure rather than a
// warning, for the live progress error count — mirroring
// cli.RenderErrors(_, true).
func isFailure(err error) bool {
	return err != nil && !types.IsWarning(err)
}
