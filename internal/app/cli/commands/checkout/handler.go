// Package checkout is the view side of `gits checkout`: it walks a project's
// repositories, asks the user which branch each should be on, and reports the
// outcome. It is not a Bulk Command: it asks the user per repository, which
// the shared command engine deliberately does not allow.
package checkout

import (
	"errors"
	"fmt"

	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/app"
	pick "github.com/rafi/gits/internal/app/cli/interaction/select"
	"github.com/rafi/gits/internal/app/cli/render/style"
	"github.com/rafi/gits/internal/runtime/command"
	"github.com/rafi/gits/internal/service/checkout"
)

// ExecCheckout display an interactive list of branches that can be checked-out.
//
// Args: (optional)
//   - project name
//   - repo or sub-project name
func ExecCheckout(tags domain.TagSet, args []string, deps app.RuntimeCLI) error {
	project, repo, err := pick.ParseArgsWithTags(args, tags, true, deps)
	if err != nil {
		return err
	}

	if repo != nil {
		// Checkout a single repository.
		err := checkoutRepo(project, *repo, deps)
		if errors.Is(err, huh.ErrUserAborted) {
			return domain.NewWarning("checkout aborted")
		}
		return err
	}

	// Checkout all project's repositories.
	errs, _ := checkoutProjectRepos(project, deps)
	if len(errs) > 0 {
		return style.RenderErrors(deps.Err, errs, true)
	}
	return nil
}

// checkoutProjectRepos walks the project tree prompting per repo. Aborting a
// prompt (Ctrl-C) stops the whole traversal instead of forcing the user to
// dismiss every remaining repository one by one.
func checkoutProjectRepos(project domain.Project, deps app.RuntimeCLI) ([]error, bool) {
	lipgloss.Fprintln(deps.Out, style.ProjectTitleWithBullet(project, deps.Theme))

	errList := make([]error, 0)
	for _, repo := range project.Repos {
		err := checkoutRepo(project, repo, deps)
		if err == nil {
			continue
		}
		if errors.Is(err, huh.ErrUserAborted) {
			return append(errList, domain.NewWarning("checkout aborted")), true
		}
		errList = append(errList, err)
	}

	for _, subProject := range project.SubProjects {
		fmt.Fprintln(deps.Out)
		errs, aborted := checkoutProjectRepos(subProject, deps)
		errList = append(errList, errs...)
		if aborted {
			return errList, true
		}
	}
	return errList, false
}

func checkoutRepo(project domain.Project, repo domain.Repository, deps app.RuntimeCLI) error {
	repoTitle := style.RepoTitle(repo, project, deps.HomeDir, deps.Theme).
		Render()

	// Abort if repository is not cloned or has errors. The title names the
	// repository the message is about, so it joins the message on Diagnostic
	// Output; AbortOnRepoState terminates the line they share.
	if repo.State != domain.RepoStateOK {
		lipgloss.Fprint(deps.Err, repoTitle)
		return style.AbortOnRepoState(deps.Err, repo, deps.Theme.Error)
	}

	want, current, err := promptRepo(repoTitle, repo, deps)
	if err != nil {
		// Name the repository, as the checkout failure below does: in a
		// project traversal these land in a summary that is otherwise anonymous.
		// The wrap keeps huh.ErrUserAborted matchable by both abort sites.
		return command.RepoError(err, repo)
	}
	if want == current {
		renderUnchanged(repoTitle, current, deps)
		return nil
	}

	if err := checkout.Switch(deps.Ctx, repo, want, deps.Runtime); err != nil {
		renderFailure(repoTitle, err, deps)
		return command.RepoError(err, repo)
	}
	renderSwitched(repoTitle, want, deps)
	return nil
}

// promptRepo prompts the user to select a branch to checkout. It returns the
// selected branch alongside the one already checked out; the two are equal
// when the selection changes nothing. The prompt itself leaves no trace on
// screen — the caller reports the outcome, once it knows what it is.
func promptRepo(repoTitle string, repo domain.Repository, deps app.RuntimeCLI) (
	string, string, error,
) {
	branches, current, err := checkout.Branches(deps.Ctx, repo, deps.Runtime)
	if err != nil {
		return "", "", err
	}

	want := current
	prompt := newBranchPrompt(repoTitle, branches, &want)
	if err := runBranchPrompt(prompt, &want); err != nil {
		return "", "", err
	}
	return want, current, nil
}
