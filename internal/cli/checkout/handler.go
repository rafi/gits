// Package checkout implements `gits checkout`, which walks a Project's
// Repositories and optionally checks a branch out of each. It is not a Bulk
// Command: it asks the user per Repository. See
// docs/adr/0005-bulk-commands-share-one-module.md.
package checkout

import (
	"errors"
	"fmt"

	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cli"
	"github.com/rafi/gits/internal/types"
)

// branchPageSize is how many branches the prompt shows at once.
const branchPageSize = 10

// ExecCheckout display an interactive list of branches that can be checked-out.
//
// Args: (optional)
//   - project name
//   - repo or sub-project name
func ExecCheckout(args []string, deps types.RuntimeCLI) error {
	project, repo, err := cli.ParseArgs(args, true, deps)
	if err != nil {
		return err
	}

	if repo != nil {
		// Checkout a single repository.
		err := checkoutRepo(project, *repo, deps)
		if errors.Is(err, huh.ErrUserAborted) {
			return types.NewWarning("checkout aborted")
		}
		return err
	}

	// Checkout all project's repositories.
	errs, _ := checkoutProjectRepos(project, deps)
	if len(errs) > 0 {
		return cli.RenderErrors(deps.Err, errs, true)
	}
	return nil
}

// checkoutProjectRepos walks the project tree prompting per repo. Aborting a
// prompt (Ctrl-C) stops the whole traversal instead of forcing the user to
// dismiss every remaining repository one by one.
func checkoutProjectRepos(project domain.Project, deps types.RuntimeCLI) ([]error, bool) {
	lipgloss.Fprintln(deps.Out, cli.ProjectTitleWithBullet(project, deps.Theme))

	errList := make([]error, 0)
	for _, repo := range project.Repos {
		err := checkoutRepo(project, repo, deps)
		if err == nil {
			continue
		}
		if errors.Is(err, huh.ErrUserAborted) {
			return append(errList, types.NewWarning("checkout aborted")), true
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

func checkoutRepo(project domain.Project, repo domain.Repository, deps types.RuntimeCLI) error {
	repoTitle := cli.RepoTitle(repo, project, deps.HomeDir, deps.Theme).
		Render()

	// Abort if repository is not cloned or has errors. The title names the
	// repository the message is about, so it joins the message on Diagnostic
	// Output; AbortOnRepoState terminates the line they share.
	if repo.State != domain.RepoStateOK {
		lipgloss.Fprint(deps.Err, repoTitle)
		return cli.AbortOnRepoState(deps.Err, repo, deps.Theme.Error)
	}

	want, current, err := promptRepo(repoTitle, repo.AbsPath, deps)
	if err != nil {
		// Name the repository, as the checkout failure below does: in a
		// project traversal these land in a summary that is otherwise anonymous.
		// The wrap keeps huh.ErrUserAborted matchable by both abort sites.
		return cli.RepoError(err, repo)
	}
	if want == current {
		lipgloss.Fprintf(deps.Out, "%s %s\n", repoTitle, current)
		return nil
	}

	err = deps.Git.Checkout(deps.Ctx, repo.AbsPath, want)
	if err != nil {
		// Titled and terminated like the two lines above: this used to trail
		// the prompt's own final line, which no longer exists.
		lipgloss.Fprintf(deps.Out, "%s %s\n", repoTitle, deps.Theme.Error.Render(err.Error()))
		return cli.RepoError(err, repo)
	}
	lipgloss.Fprintf(deps.Out, "%s %s\n", repoTitle, deps.Theme.GitOutput.Render(
		fmt.Sprintf("Switched to branch %q", want),
	))
	return nil
}

// promptRepo prompts the user to select a branch to checkout. It returns the
// selected branch alongside the one already checked out; the two are equal
// when the selection changes nothing. The prompt itself leaves no trace on
// screen — the caller reports the outcome, once it knows what it is.
func promptRepo(repoTitle, repoPath string, deps types.RuntimeCLI) (string, string, error) {
	current, err := deps.Git.CurrentBranch(deps.Ctx, repoPath)
	if err != nil {
		return "", "", fmt.Errorf("unable to get branch: %w", err)
	}

	branches, err := deps.Git.AllBranches(deps.Ctx, repoPath)
	if err != nil {
		return "", "", fmt.Errorf("unable to read branches: %w", err)
	}
	// An option-less select cannot be submitted, only aborted. Defensive: a
	// repository with no commits already failed above, in CurrentBranch.
	if len(branches) == 0 {
		return "", "", errors.New("repository has no branches")
	}

	want := current
	prompt := newBranchPrompt(repoTitle, branches, &want)
	if err := runBranchPrompt(prompt, &want); err != nil {
		return "", "", err
	}
	return want, current, nil
}

// runBranchPrompt runs the branch selection and is the package's one
// interactive seam: everything around it — the state guard, the three outcome
// lines a repository gets once its prompt returns, the traversal and the error
// epilogue — is reachable in a test by replacing this, without a terminal.
//
// choice is the same binding the prompt was built with, passed explicitly so a
// replacement can report a selection by writing through it, exactly as huh
// does. It is unused here.
//
// The field runs as a form rather than through prompt.Run(), which hides the
// help line: it is the only place "/" is advertised as the filter key.
//
// The form is deliberately not given a context, unlike every git call around
// it. Bubbletea handles SIGINT itself and reports it as an abort, which is
// what an interrupted prompt is; handing it the root context would reach the
// same user through huh.ErrTimeout instead.
var runBranchPrompt = func(prompt *huh.Select[string], _ *string) error {
	return huh.NewForm(huh.NewGroup(prompt)).Run()
}

// newBranchPrompt builds the branch selection. choice is huh's value binding:
// the caller seeds it with the branch currently checked out — which the prompt
// opens on, and titles itself with — and it holds the user's pick once the
// prompt has run.
func newBranchPrompt(repoTitle string, branches []string, choice *string) *huh.Select[string] {
	prompt := huh.NewSelect[string]().
		Title(fmt.Sprintf("%s [%s]", repoTitle, *choice)).
		Options(huh.NewOptions(branches...)...).
		Value(choice)

	// Only ask for a height when the list needs paging. huh pads a field out
	// to the height it is given — blank rows a project-wide run would repeat
	// for every repository — whereas an unset height sizes the field to its
	// options exactly, however the title happens to wrap. The extra row is
	// that title, which counts against the height.
	if len(branches) > branchPageSize {
		prompt = prompt.Height(branchPageSize + 1)
	}
	return prompt
}
