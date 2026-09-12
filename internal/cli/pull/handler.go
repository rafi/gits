package pull

import (
	"context"
	"fmt"

	"charm.land/lipgloss/v2"

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

	if repo != nil {
		// Pull a single repository.
		res := pullRepo(deps.Ctx, project, *repo, deps)
		lipgloss.Println(cli.IndentMultiline(res.Line))
		return res.Err
	}

	// Pull all project repositories through the shared walker.
	errs := walk.Walk(deps.Ctx, project, deps, "pulling", pullRepo)
	return cli.RenderErrors(errs, true)
}

type PullResponse struct {
	currentBranch string
	upstream      string
	output        string
	title         lipgloss.Style
	error         error
	errorStyle    lipgloss.Style
}

func (r PullResponse) String() string {
	if r.error != nil {
		return fmt.Sprintf("%s %s", r.title, r.errorStyle.Render(r.error.Error()))
	}

	return fmt.Sprintf(
		"%s [%s <- %s] %s",
		r.title.Render(),
		r.currentBranch,
		r.upstream,
		r.output,
	)
}

// pullRepo pulls one repository and returns its rendered result. It is a
// walk.RepoFunc: safe to call concurrently and never writes to stdout.
func pullRepo(
	ctx context.Context,
	project domain.Project,
	repo domain.Repository,
	deps types.RuntimeCLI,
) walk.RepoResult {
	resp := PullResponse{
		title:      cli.PaddedRepoTitle(repo, project, deps),
		errorStyle: deps.Theme.Error,
	}

	// Abort if repository is not cloned or has errors.
	if repo.State != domain.RepoStateOK {
		resp.error = cli.RepoStateWarning(repo)
		return walk.RepoResult{Line: resp.String(), Err: resp.error}
	}

	var err error
	resp.currentBranch, err = deps.Git.CurrentBranch(ctx, repo.AbsPath)
	if err != nil {
		resp.error = cli.RepoError(err, repo)
		return walk.RepoResult{Line: resp.String(), Err: resp.error}
	}

	resp.upstream, err = deps.Git.UpstreamBranch(ctx, repo.AbsPath)
	if err != nil || resp.upstream == "" {
		resp.error = cli.RepoError(git.ErrNoUpstream, repo)
		return walk.RepoResult{Line: resp.String(), Err: resp.error}
	}

	resp.output, err = deps.Git.Pull(ctx, repo.AbsPath)
	if err != nil {
		resp.error = cli.RepoError(err, repo)
		return walk.RepoResult{Line: resp.String(), Err: resp.error}
	}
	resp.output = deps.Theme.GitOutput.Render(resp.output)
	return walk.RepoResult{Line: resp.String(), Err: nil}
}
