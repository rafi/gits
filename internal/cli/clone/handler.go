package clone

import (
	"context"
	"errors"

	log "github.com/sirupsen/logrus"

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

	if repo != nil {
		fn := cloneRepo(cli.NewTitleWidths(project, deps.HomeDir))
		return walk.Single(deps.Ctx, project, *repo, deps, fn)
	}

	// Clone all project repositories through the shared walker. Projects with
	// clone disabled are pruned first so the walker never queues their repos.
	pruned := pruneSkipped(project)
	fn := cloneRepo(cli.NewTitleWidths(pruned, deps.HomeDir))
	errs := walk.Walk(deps.Ctx, pruned, deps, "cloning", fn)
	return cli.RenderErrors(errs, true)
}

// pruneSkipped returns a copy of the project tree with clone-disabled projects
// emptied of their repos and sub-projects, preserving the historical behavior
// where `clone: false` skips a project and everything beneath it. The original
// tree is left unmodified.
func pruneSkipped(p domain.Project) domain.Project {
	if p.Clone != nil && !*p.Clone {
		log.Warn("Skipping clone due to config")
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
			return walk.LineResult(line, cli.RepoStateWarning(repo))
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
