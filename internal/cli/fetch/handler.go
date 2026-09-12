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

type FetchResponse struct {
	repoPath   string
	output     string
	title      lipgloss.Style
	error      error
	errorStyle lipgloss.Style
}

func (r FetchResponse) String() string {
	if r.error != nil {
		return fmt.Sprintf("%s %s", r.title, r.errorStyle.Render(r.error.Error()))
	}

	if r.title.Value() == r.repoPath {
		return fmt.Sprintf("%s %s", r.title, r.output)
	}
	return fmt.Sprintf("%s %s %s", r.title, r.repoPath, r.output)
}

// fetchRepo fetches one repository and returns its rendered result. It is a
// walk.RepoFunc: safe to call concurrently and never writes to stdout.
func fetchRepo(
	ctx context.Context,
	project domain.Project,
	repo domain.Repository,
	deps types.RuntimeCLI,
) walk.RepoResult {
	resp := FetchResponse{
		title:      cli.PaddedRepoTitle(repo, project, deps),
		repoPath:   cli.Path(repo.AbsPath, deps.HomeDir),
		errorStyle: deps.Theme.Error,
	}

	// Abort if repository is not cloned or has errors.
	if repo.State != domain.RepoStateOK {
		resp.error = cli.RepoStateWarning(repo)
		return walk.RepoResult{Line: resp.String(), Err: resp.error}
	}

	var err error
	resp.output, err = deps.Git.Fetch(ctx, repo.AbsPath)
	if err != nil {
		resp.error = cli.RepoError(err, repo)
		return walk.RepoResult{Line: resp.String(), Err: resp.error}
	}
	resp.output = deps.Theme.GitOutput.Render(resp.output)
	return walk.RepoResult{Line: resp.String(), Err: nil}
}
