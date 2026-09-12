package status

import (
	"context"
	"fmt"

	"charm.land/lipgloss/v2"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cli"
	"github.com/rafi/gits/internal/cli/walk"
	"github.com/rafi/gits/internal/types"
)

// ExecStatus displays an icon based status of all repositories.
//
// Args: (optional)
//   - project name
//   - repo or sub-project name
func ExecStatus(args []string, deps types.RuntimeCLI) error {
	project, repo, err := cli.ParseArgs(args, true, deps)
	if err != nil {
		return err
	}

	if repo != nil {
		// Display status for a single repository.
		res := statusRepo(deps.Ctx, project, *repo, deps)
		lipgloss.Println(cli.IndentMultiline(res.Line))
		return res.Err
	}

	// Display status for all project repositories through the shared walker,
	// which buffers each repo's line and prints them in stable tree order.
	errs := walk.Walk(deps.Ctx, project, deps, "checking status", statusRepo)
	return cli.RenderErrors(errs, true)
}

// statusRepo builds one repository's status line and returns it. It is a
// walk.RepoFunc: safe to call concurrently and never writes to stdout, so the
// per-repo git probes no longer race each other onto the terminal.
func statusRepo(
	ctx context.Context,
	project domain.Project,
	repo domain.Repository,
	deps types.RuntimeCLI,
) walk.RepoResult {
	// Status is several quick local probes; the row spins until the line is
	// ready (AC-4).
	title := cli.PaddedRepoTitle(repo, project, deps).Align(lipgloss.Right)

	// Abort if repository is not cloned or has errors.
	if repo.State != domain.RepoStateOK {
		errStyle := deps.Theme.Error.PaddingLeft(8)
		line, err := cli.RepoStateError(repo, errStyle)
		return walk.RepoResult{Line: fmt.Sprintf("%s %s", title, line), Err: err}
	}

	version, err := deps.Git.Describe(ctx, repo.AbsPath)
	if err != nil {
		version = ""
	}

	var count int
	modified := ""
	if count, err = deps.Git.Modified(ctx, repo.AbsPath); err != nil {
		return statusError(title, repo, err, deps)
	} else if count > 0 {
		modified = fmt.Sprintf("≠%d", count)
	}
	untracked := ""
	if count, err = deps.Git.Untracked(ctx, repo.AbsPath); err != nil {
		return statusError(title, repo, err, deps)
	} else if count > 0 {
		untracked = fmt.Sprintf("?%d", count)
	}

	branch, _ := deps.Git.CurrentBranch(ctx, repo.AbsPath)
	upstream, err := deps.Git.UpstreamBranch(ctx, repo.AbsPath)
	if err != nil || upstream == "" {
		// No upstream configured: compare against the conventional remote branch.
		upstream = fmt.Sprintf("origin/%v", branch)
	}

	diff := ""
	ahead, behind, err := deps.Git.Diff(ctx, repo.AbsPath, branch, upstream)
	switch {
	case err != nil:
		diff = "-"
	case ahead == 0 && behind == 0:
		diff = "✓"
	default:
		if ahead > 0 {
			diff = fmt.Sprintf("▲%d", ahead)
		}
		if behind > 0 {
			diff = fmt.Sprintf("%s▼%d", diff, behind)
		}
	}

	currentRef, err := deps.Git.CurrentPosition(ctx, repo.AbsPath)
	if err != nil {
		currentRef = "N/A"
	}

	body := fmt.Sprintf("%s %s %s %s %s",
		deps.Theme.Modified.Render(modified),
		deps.Theme.Untracked.Render(untracked),
		deps.Theme.Diff.Render(diff),
		version,
		currentRef,
	)
	return walk.RepoResult{Line: fmt.Sprintf("%s %s", title, body)}
}

// statusError renders a status line whose body is a failed git probe.
func statusError(
	title lipgloss.Style,
	repo domain.Repository,
	err error,
	deps types.RuntimeCLI,
) walk.RepoResult {
	wrapped := cli.RepoError(err, repo)
	return walk.RepoResult{
		Line: fmt.Sprintf("%s %s", title, deps.Theme.Error.Render(err.Error())),
		Err:  wrapped,
	}
}
