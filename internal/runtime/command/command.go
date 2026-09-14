// Package command executes one operation across many repositories: it takes a
// resolved target, applies the command's own pruning and state guard, and
// works through the tree with a bounded worker pool, handing back one result
// per repository in stable tree order.
//
// Nothing here renders. Progress is an interface a caller may implement, and
// the results are data a view turns into lines, a table or a document.
package command

import (
	"context"
	"slices"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/format"
	coreruntime "github.com/rafi/gits/internal/runtime"
)

// Command is one operation applied across repositories. T is whatever its
// body produces for each of them: the body text for the commands rendering
// lines, a structured value for one rendering its own output.
type Command[T any] struct {
	// Name is the command's own name ("pull"): the key its per-repository
	// outcome nests under in the JSON envelope. A command that never renders
	// JSON may leave it empty.
	Name string
	// Verb labels the run in the progress reporter ("pulling").
	Verb string
	// Accepts are the Repo States the body is willing to receive. A nil set
	// means the ok state: the permissive default's failure mode is a body
	// dereferencing a path that is not there, which is what the guard exists
	// to prevent. A repository failing the guard carries its state error as
	// its result and never reaches the body.
	Accepts []domain.RepoState
	// Skip drops a project and everything beneath it before the command. Unlike
	// the guard, a skipped project vanishes: never queued, never rendered,
	// and absent from the progress reporter's total. A nil Skip visits
	// everything.
	Skip func(domain.Project) bool
	// Do does one repository's work. It must be safe to call concurrently
	// and must write to no destination itself. The error it returns is kept
	// as it is: shown bare on the repository's own line, and wrapped with
	// the repository's name and path only in the error epilogue. A warning
	// built with domain.NewWarning is a documented pass-over — it shows on
	// the line and does not fail the command.
	Do func(context.Context, Repo, coreruntime.Runtime) (T, error)
	// Progress receives the live lifecycle of the command. A nil Progress is the
	// no-op: the engine reports nothing anywhere by itself.
	Progress Progress
}

// Target is what a run was pointed at: a project tree, or one repository
// within it when Repo is set.
type Target struct {
	Project domain.Project
	Repo    *domain.Repository
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
	// for the home directory. A line renderer pads it to the widest in the
	// project.
	Path string
}

// Result is one repository's outcome. Value is the body's own, so no caller
// asserts a type at this seam; Err is exactly what the body or the state
// guard returned.
type Result[T any] struct {
	Repo  Repo
	Value T
	Err   error
	// Guarded reports that the state guard turned the repository back: Err
	// is its state error, and the body never ran. A renderer that
	// distinguishes "the command failed here" from "the command was never
	// tried here" reads this rather than re-deriving it from the state,
	// since which states are acceptable is the command's to declare.
	Guarded bool
}

// Results is everything a renderer receives: the tree the run visited, one
// result per repository that was started, in stable tree order, and the
// interruption error when the run was cut short. A repository the run never
// started — canceled before it was dequeued — has no result; Interrupted
// says how many there were.
type Results[T any] struct {
	// Command is the Name of the command that ran.
	Command string
	// Project is the pruned tree the run visited. For a single named
	// repository it holds that repository alone, with no sub-projects. It is
	// the zero value when the named project was skipped.
	Project domain.Project
	Results []Result[T]
	// Skipped names the projects the command's own Skip dropped, in the
	// order they were met. A view reports them so a project that vanished is
	// distinguishable from an empty one.
	Skipped     []string
	Interrupted error
}

// Run executes the command over the target. Everything that could fail before
// any repository is touched — naming a project, choosing a repository — has
// already happened, so a failure here belongs to a repository and travels in
// its result.
func (c Command[T]) Run(target Target, rt coreruntime.Runtime) Results[T] {
	// Pruning precedes the dispatch so a skipped project is skipped on both
	// paths: how the command was invoked must not override its configuration.
	var skipped []string
	project, kept := c.prune(target.Project, &skipped)
	if !kept {
		// The named project is skipped, so there is nothing to command.
		return Results[T]{Command: c.Name, Skipped: skipped}
	}

	var res Results[T]
	if target.Repo != nil {
		res = c.single(rt.Ctx, project, *target.Repo, rt)
	} else {
		res = c.collect(rt.Ctx, project, rt)
	}
	res.Command = c.Name
	res.Skipped = skipped
	return res
}

// single runs the body for one named repository. The tree it reports is the
// project narrowed to that repository, so a renderer walks the same shape it
// would for a whole command. A repository whose project was pruned yields no
// result at all: it is not run, and nothing is rendered for it.
func (c Command[T]) single(
	ctx context.Context,
	project domain.Project,
	repo domain.Repository,
	rt coreruntime.Runtime,
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
	res := c.one(ctx, newRepo(repo, project, "0", rt.HomeDir), rt)
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
		Path:       format.RepoRelPath(project, repo, homeDir),
	}
}

// one applies the state guard and, when it passes, the body — the whole of
// what happens to a single repository, on either dispatch path.
func (c Command[T]) one(ctx context.Context, repo Repo, rt coreruntime.Runtime) Result[T] {
	res := Result[T]{Repo: repo}
	if !c.accepts(repo.State) {
		res.Err = StateError(repo.Repository)
		res.Guarded = true
		return res
	}
	res.Value, res.Err = c.Do(ctx, repo, rt)
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

// prune returns the tree the run visits and whether p itself survived,
// appending every skipped project's name to skipped. A skipped project is
// dropped whole — its repositories, its sub-projects and its own title —
// which is what makes it vanish rather than appear as an empty block. The
// original tree is left unmodified.
func (c Command[T]) prune(p domain.Project, skipped *[]string) (domain.Project, bool) {
	if c.Skip == nil {
		return p, true
	}
	if c.Skip(p) {
		// Naming it once is enough: the whole subtree goes with it.
		*skipped = append(*skipped, p.Name)
		return p, false
	}
	subs := make([]domain.Project, 0, len(p.SubProjects))
	for _, sub := range p.SubProjects {
		if kept, ok := c.prune(sub, skipped); ok {
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
// warning, for the progress error count.
func isFailure(err error) bool {
	return err != nil && !domain.IsWarning(err)
}
