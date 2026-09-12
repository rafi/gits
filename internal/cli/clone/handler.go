package clone

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cli"
	"github.com/rafi/gits/internal/cli/walk"
	"github.com/rafi/gits/internal/types"
	"github.com/rafi/gits/pkg/git"
)

// ExecClone clones project repositories, or a specific repo.
//
// Args: (optional)
//   - project name
//   - repo or sub-project name
func ExecClone(args []string, deps types.RuntimeCLI) error {
	project, repo, err := cli.ParseArgs(args, true, deps)
	if err != nil {
		return err
	}

	// Projects with clone disabled are pruned before either path is taken, so
	// how the command was invoked cannot override the configuration: naming a
	// repository explicitly must not clone one the project said to skip.
	reportSkipped(deps.Err, project)
	pruned := pruneSkipped(project)
	fn := cloneRepo(cli.NewTitleWidths(pruned, deps.HomeDir))

	if repo != nil {
		if !inTree(pruned, *repo) {
			return nil
		}
		return walk.Single(deps.Ctx, pruned, *repo, deps, fn)
	}

	// Clone all project repositories through the shared walker.
	errs := walk.Walk(deps.Ctx, pruned, deps, "cloning", fn)
	return cli.RenderErrors(deps.Err, errs, true)
}

// pruneSkipped returns a copy of the project tree with clone-disabled projects
// emptied of their repos and sub-projects, preserving the historical behavior
// where `clone: false` skips a project and everything beneath it. The original
// tree is left unmodified.
func pruneSkipped(p domain.Project) domain.Project {
	if skips(p) {
		p.Repos = nil
		p.SubProjects = nil
		return p
	}
	subs := make([]domain.Project, len(p.SubProjects))
	for i := range p.SubProjects {
		subs[i] = pruneSkipped(p.SubProjects[i])
	}
	p.SubProjects = subs
	return p
}

// reportSkipped names every clone-disabled project as Diagnostic Output, so a
// project that vanishes from the run is distinguishable from an empty one. It
// does not descend past a skipped project: the whole subtree goes with it, and
// saying so once is the message.
func reportSkipped(w io.Writer, p domain.Project) {
	if skips(p) {
		fmt.Fprintf(w, "Skipping %s: clone disabled in config\n", p.Name)
		return
	}
	for _, sub := range p.SubProjects {
		reportSkipped(w, sub)
	}
}

// skips reports whether a project's configuration disables cloning it.
func skips(p domain.Project) bool {
	return p.Clone != nil && !*p.Clone
}

// inTree reports whether repo survived pruning — its owning project is not
// clone-disabled — identifying it by the local path the loader resolved for
// it, which is unique across a project tree.
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

// cloneRepo returns a walk.RepoFunc that clones one repository into a rendered
// result: safe to call concurrently and never writes to stdout. Unlike the
// other bulk commands it only rejects repos in an error state, since a
// not-yet-cloned repo is the expected input here.
func cloneRepo(widths cli.TitleWidths) walk.RepoFunc {
	return func(
		ctx context.Context,
		project domain.Project,
		repo domain.Repository,
		deps types.RuntimeCLI,
	) walk.RepoResult {
		line := cli.RepoLine{
			Title:      cli.PaddedRepoTitle(repo, project, widths.For(project), deps),
			ErrorStyle: deps.Theme.Error,
		}

		if repo.State == domain.RepoStateError {
			return walk.LineResult(line, cli.RepoStateError(repo))
		}

		output, err := deps.Git.Clone(ctx, repo.Src, repo.AbsPath)
		if errors.Is(err, git.ErrTargetExists) {
			repoPath := cli.Path(repo.AbsPath, deps.HomeDir)
			return walk.LineResult(line, types.NewWarning("already cloned at %s", repoPath))
		}
		if err != nil {
			return walk.LineResult(line, cli.RepoError(err, repo))
		}
		line.Body = deps.Theme.GitOutput.Render(output)
		return walk.LineResult(line, nil)
	}
}
