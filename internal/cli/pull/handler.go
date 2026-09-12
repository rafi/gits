package pull

import (
	"context"
	"errors"
	"fmt"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cli"
	"github.com/rafi/gits/internal/cli/walk"
	"github.com/rafi/gits/internal/types"
	"github.com/rafi/gits/pkg/git"
)

// ExecPull runs pull --ff-only on project repositories, or on a specific repo.
//
// Args: (optional)
//   - project name
//   - repo
func ExecPull(args []string, deps types.RuntimeCLI) error {
	project, repo, err := cli.ParseArgs(args, true, deps)
	if err != nil {
		return err
	}

	fn := pullRepo(cli.NewTitleWidths(project, deps.HomeDir))

	if repo != nil {
		return walk.Single(deps.Ctx, project, *repo, deps, fn)
	}

	// Pull all project repositories through the shared walker.
	errs := walk.Walk(deps.Ctx, project, deps, "pulling", fn)
	return cli.RenderErrors(errs, true)
}

// pullRepo returns a walk.RepoFunc that pulls one repository into a rendered
// result: safe to call concurrently and never writes to stdout.
func pullRepo(widths cli.TitleWidths) walk.RepoFunc {
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

		// Abort if repository is not cloned or has errors.
		if repo.State != domain.RepoStateOK {
			return walk.LineResult(line, cli.RepoStateWarning(repo))
		}

		currentBranch, err := deps.Git.CurrentBranch(ctx, repo.AbsPath)
		if err != nil {
			return walk.LineResult(line, cli.RepoError(err, repo))
		}

		upstream, err := deps.Git.UpstreamBranch(ctx, repo.AbsPath)
		if err != nil && !errors.Is(err, git.ErrNoUpstream) {
			// A real failure (e.g. cancellation) keeps its own message instead
			// of being mislabeled as a missing upstream.
			return walk.LineResult(line, cli.RepoError(err, repo))
		}
		if err != nil || upstream == "" {
			return walk.LineResult(line, cli.RepoError(git.ErrNoUpstream, repo))
		}

		output, err := deps.Git.Pull(ctx, repo.AbsPath)
		if err != nil {
			return walk.LineResult(line, cli.RepoError(err, repo))
		}
		line.Body = fmt.Sprintf(
			"[%s <- %s] %s",
			currentBranch,
			upstream,
			deps.Theme.GitOutput.Render(output),
		)
		return walk.LineResult(line, nil)
	}
}
