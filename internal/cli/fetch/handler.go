package fetch

import (
	"context"
	"fmt"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cli"
	"github.com/rafi/gits/internal/cli/walk"
	"github.com/rafi/gits/internal/types"
)

// ExecFetch runs fetch on project repositories, or on a specific repo.
//
// Args: (optional)
//   - project name
//   - repo or sub-project name
func ExecFetch(args []string, deps types.RuntimeCLI) error {
	project, repo, err := cli.ParseArgs(args, true, deps)
	if err != nil {
		return err
	}

	fn := fetchRepo(cli.NewTitleWidths(project, deps.HomeDir))

	if repo != nil {
		return walk.Single(deps.Ctx, project, *repo, deps, fn)
	}

	// Fetch all project repositories through the shared walker.
	errs := walk.Walk(deps.Ctx, project, deps, "fetching", fn)
	return cli.RenderErrors(deps.Err, errs, true)
}

// fetchRepo returns a walk.RepoFunc that fetches one repository into a
// rendered result: safe to call concurrently and never writes to stdout.
func fetchRepo(widths cli.TitleWidths) walk.RepoFunc {
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
		repoPath := cli.Path(repo.AbsPath, deps.HomeDir)

		// Abort if repository is not cloned or has errors.
		if repo.State != domain.RepoStateOK {
			return walk.LineResult(line, cli.RepoStateError(repo))
		}

		output, err := deps.Git.Fetch(ctx, repo.AbsPath)
		if err != nil {
			return walk.LineResult(line, cli.RepoError(err, repo))
		}
		line.Body = deps.Theme.GitOutput.Render(output)
		if line.Title.Value() != repoPath {
			line.Body = fmt.Sprintf("%s %s", repoPath, line.Body)
		}
		return walk.LineResult(line, nil)
	}
}
