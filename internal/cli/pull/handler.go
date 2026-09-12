package pull

import (
	"context"
	"errors"
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

// pullRepo pulls one repository and returns its rendered result. It is a
// walk.RepoFunc: safe to call concurrently and never writes to stdout.
func pullRepo(
	ctx context.Context,
	project domain.Project,
	repo domain.Repository,
	deps types.RuntimeCLI,
) walk.RepoResult {
	line := cli.RepoLine{
		Title:      cli.PaddedRepoTitle(repo, project, deps),
		ErrorStyle: deps.Theme.Error,
	}

	// Abort if repository is not cloned or has errors.
	if repo.State != domain.RepoStateOK {
		line.Err = cli.RepoStateWarning(repo)
		return walk.RepoResult{Line: line.String(), Err: line.Err}
	}

	currentBranch, err := deps.Git.CurrentBranch(ctx, repo.AbsPath)
	if err != nil {
		line.Err = cli.RepoError(err, repo)
		return walk.RepoResult{Line: line.String(), Err: line.Err}
	}

	upstream, err := deps.Git.UpstreamBranch(ctx, repo.AbsPath)
	if err != nil && !errors.Is(err, git.ErrNoUpstream) {
		// A real failure (e.g. cancellation) keeps its own message instead
		// of being mislabeled as a missing upstream.
		line.Err = cli.RepoError(err, repo)
		return walk.RepoResult{Line: line.String(), Err: line.Err}
	}
	if err != nil || upstream == "" {
		line.Err = cli.RepoError(git.ErrNoUpstream, repo)
		return walk.RepoResult{Line: line.String(), Err: line.Err}
	}

	output, err := deps.Git.Pull(ctx, repo.AbsPath)
	if err != nil {
		line.Err = cli.RepoError(err, repo)
		return walk.RepoResult{Line: line.String(), Err: line.Err}
	}
	line.Body = fmt.Sprintf(
		"[%s <- %s] %s",
		currentBranch,
		upstream,
		deps.Theme.GitOutput.Render(output),
	)
	return walk.RepoResult{Line: line.String(), Err: nil}
}
