package fetch

import (
	"context"
	"fmt"

	"charm.land/lipgloss/v2"

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

	if repo != nil {
		// Fetch a single repository.
		res := fetchRepo(deps.Ctx, project, *repo, deps)
		lipgloss.Println(cli.IndentMultiline(res.Line))
		return res.Err
	}

	// Fetch all project repositories through the shared walker.
	errs := walk.Walk(deps.Ctx, project, deps, "fetching", fetchRepo)
	return cli.RenderErrors(errs, true)
}

// fetchRepo fetches one repository and returns its rendered result. It is a
// walk.RepoFunc: safe to call concurrently and never writes to stdout.
func fetchRepo(
	ctx context.Context,
	project domain.Project,
	repo domain.Repository,
	deps types.RuntimeCLI,
) walk.RepoResult {
	line := cli.RepoLine{
		Title:      cli.PaddedRepoTitle(repo, project, deps),
		ErrorStyle: deps.Theme.Error,
	}
	repoPath := cli.Path(repo.AbsPath, deps.HomeDir)

	// Abort if repository is not cloned or has errors.
	if repo.State != domain.RepoStateOK {
		line.Err = cli.RepoStateWarning(repo)
		return walk.RepoResult{Line: line.String(), Err: line.Err}
	}

	output, err := deps.Git.Fetch(ctx, repo.AbsPath)
	if err != nil {
		line.Err = cli.RepoError(err, repo)
		return walk.RepoResult{Line: line.String(), Err: line.Err}
	}
	line.Body = deps.Theme.GitOutput.Render(output)
	if line.Title.Value() != repoPath {
		line.Body = fmt.Sprintf("%s %s", repoPath, line.Body)
	}
	return walk.RepoResult{Line: line.String(), Err: nil}
}
