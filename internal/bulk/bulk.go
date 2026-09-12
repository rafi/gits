// Package bulk runs a Bulk Command: one operation applied to every repository
// in a project tree, reporting the outcome per repository.
//
// A command declares five things — a verb, the Repo States it acts on, an
// optional project skip, a per-repository body and a renderer — and Run owns
// everything else: argument resolution and interactive selection, pruning,
// dispatch between the whole tree and a single repository, the Repo State
// guard, title measuring and padding, the bounded worker pool with live
// progress, stable tree ordering, interruption accounting, and per-repository
// error wrapping.
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

	"charm.land/lipgloss/v2"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cli"
	"github.com/rafi/gits/internal/types"
)

// Command is one Bulk Command. T is whatever its body produces for each
// repository: the rendered body text for the commands using the Lines
// renderer, a structured value for one supplying its own.
type Command[T any] struct {
	// Verb labels the run in the live progress reporter ("pulling").
	Verb string
	// Accepts are the Repo States the body is willing to receive. A nil set
	// means the ok state: the permissive default's failure mode is a body
	// dereferencing a path that is not there, which is what the guard exists
	// to prevent. A repository failing the guard renders its state error and
	// never reaches the body.
	Accepts []domain.RepoState
	// Skip drops a project and everything beneath it before the run. Unlike
	// the guard, a skipped project vanishes: never queued, never rendered —
	// not even its title — and absent from the progress reporter's total. A
	// nil Skip visits everything.
	Skip func(domain.Project) bool
	// Body does one repository's work. It must be safe to call concurrently
	// and must write to no destination itself. A plain error it returns is
	// wrapped with the repository's name and path; an already-wrapped warning
	// passes through untouched, which is how a documented pass-over condition
	// says it is not a failure.
	Body func(context.Context, Repo, types.RuntimeCLI) (T, error)
	// Render turns the collected results into Result Output and returns the
	// command's error. Lines is the stock renderer for line-per-repository
	// output.
	Render func(Results[T], types.RuntimeCLI) error
}

// Repo is the bundled per-repository argument a body receives: the repository,
// its owning project, and the title the module already measured, so no body
// computes one and none can compute it wrongly.
type Repo struct {
	domain.Repository

	// Project is the repository's owning project — the resolved project
	// itself when a single repository was named.
	Project domain.Project
	// Title is the repository's display path, padded to the widest in its
	// project so result bodies align. Its Value() is the unpadded path.
	Title lipgloss.Style
}

// Result is one repository's outcome. Value is the body's own, so no caller
// asserts a type at this seam and no row is dropped when such an assertion
// would have failed.
type Result[T any] struct {
	Repo  Repo
	Value T
	Err   error
}

// Group is one project's results in stable tree order. A nil slot means the
// repository was never started — canceled before it was dequeued.
type Group[T any] struct {
	Project domain.Project
	Results []*Result[T]
}

// Results is everything a renderer receives: the groups in tree order, the
// interruption error when the run was cut short, and whether the run was for a
// single named repository. Single carries what used to be implicit in which
// entry point a command called — whether project titles are drawn, and whether
// a warning reaches the root as itself or is filtered by the error epilogue.
type Results[T any] struct {
	Groups      []Group[T]
	Interrupted error
	Single      bool
}

// Run resolves the arguments — prompting for a project or repository when they
// are missing — and executes the command over what they named.
func (c Command[T]) Run(args []string, deps types.RuntimeCLI) error {
	project, repo, err := cli.ParseArgs(args, true, deps)
	if err != nil {
		return err
	}

	// Pruning precedes the dispatch so a skipped project is skipped on both
	// paths: how the command was invoked must not override its configuration.
	project, kept := c.prune(project, deps.Err)
	if !kept {
		// The named project is skipped, so there is nothing to run and no
		// group to render. prune has already said so as Diagnostic Output.
		return c.Render(Results[T]{Single: repo != nil}, deps)
	}
	widths := newTitleWidths(project, deps.HomeDir)

	res := Results[T]{Single: repo != nil}
	if repo != nil {
		res.Groups = c.single(deps.Ctx, project, *repo, widths, deps)
	} else {
		res.Groups, res.Interrupted = c.collect(deps.Ctx, project, widths, deps)
	}
	return c.Render(res, deps)
}

// single runs the body for one named repository, in the one-group shape a
// renderer receives for a whole tree. A repository whose project was pruned
// yields no group at all: it is not run, and nothing is rendered for it.
func (c Command[T]) single(
	ctx context.Context,
	project domain.Project,
	repo domain.Repository,
	widths titleWidths,
	deps types.RuntimeCLI,
) []Group[T] {
	// Argument resolution found this repository in the tree, and pruning
	// shares every surviving project's Repos array — so the only way it is
	// missing now is that Skip dropped a project between it and the root,
	// which prune has already reported. Nothing to run, and nothing failed.
	if !inTree(project, repo) {
		return nil
	}
	res := c.one(ctx, Repo{
		Repository: repo,
		Project:    project,
		Title:      paddedRepoTitle(repo, project, widths.For(project), deps),
	}, deps)
	return []Group[T]{{Project: project, Results: []*Result[T]{&res}}}
}

// one applies the state guard and, when it passes, the body — the whole of
// what happens to a single repository, on either dispatch path.
func (c Command[T]) one(ctx context.Context, repo Repo, deps types.RuntimeCLI) Result[T] {
	res := Result[T]{Repo: repo}
	if !c.accepts(repo.State) {
		res.Err = cli.RepoStateError(repo.Repository)
		return res
	}
	res.Value, res.Err = c.Body(ctx, repo, deps)
	res.Err = wrapRepoError(res.Err, repo.Repository)
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

// wrapRepoError attaches the repository's name and path to a body's error, so
// no body repeats that call at every failure. An error a body already wrapped
// itself — the downgraded form for a condition the command is documented to
// pass over — is left exactly as it is.
func wrapRepoError(err error, repo domain.Repository) error {
	var wrapped *types.Warning
	if err == nil || errors.As(err, &wrapped) {
		return err
	}
	return cli.RepoError(err, repo)
}

// inTree reports whether repo survived pruning, identifying it by the local
// path the loader resolved for it, which is unique across a project tree.
func inTree(p domain.Project, repo domain.Repository) bool {
	for _, r := range p.Repos {
		if r.AbsPath == repo.AbsPath && r.GetName() == repo.GetName() {
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
