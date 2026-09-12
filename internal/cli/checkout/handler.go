package checkout

import (
	"errors"
	"fmt"

	"charm.land/lipgloss/v2"
	"github.com/erikgeiser/promptkit"
	"github.com/erikgeiser/promptkit/selection"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cli"
	"github.com/rafi/gits/internal/types"
)

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
		if errors.Is(err, promptkit.ErrAborted) {
			return types.NewWarning("checkout aborted")
		}
		return err
	}

	// Checkout all project's repositories.
	errs, _ := checkoutProjectRepos(project, deps)
	if len(errs) > 0 {
		return cli.RenderErrors(errs, true)
	}
	return nil
}

// checkoutProjectRepos walks the project tree prompting per repo. Aborting a
// prompt (Esc/Ctrl-C) stops the whole traversal instead of forcing the user
// to dismiss every remaining repository one by one.
func checkoutProjectRepos(project domain.Project, deps types.RuntimeCLI) ([]error, bool) {
	lipgloss.Println(cli.ProjectTitleWithBullet(project, deps.Theme))

	errList := make([]error, 0)
	for _, repo := range project.Repos {
		err := checkoutRepo(project, repo, deps)
		if err == nil {
			continue
		}
		if errors.Is(err, promptkit.ErrAborted) {
			return append(errList, types.NewWarning("checkout aborted")), true
		}
		errList = append(errList, err)
	}

	for _, subProject := range project.SubProjects {
		fmt.Println()
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

	// Abort if repository is not cloned or has errors.
	if repo.State != domain.RepoStateOK {
		lipgloss.Print(repoTitle)
		defer fmt.Println()
		return cli.AbortOnRepoState(repo, deps.Theme.Error)
	}

	branch, err := promptRepo(repoTitle, repo.AbsPath, deps)
	if err != nil {
		return err
	}
	if branch == "" {
		return nil
	}

	err = deps.Git.Checkout(deps.Ctx, repo.AbsPath, branch)
	if err != nil {
		lipgloss.Print(deps.Theme.Error.Render(err.Error()))
		return cli.RepoError(err, repo)
	}
	return nil
}

// promptRepo prompts the user to select a branch to checkout.
func promptRepo(repoTitle, repoPath string, deps types.RuntimeCLI) (string, error) {
	current, err := deps.Git.CurrentBranch(deps.Ctx, repoPath)
	if err != nil {
		return "", fmt.Errorf("unable to get branch: %w", err)
	}

	ps := fmt.Sprintf("%s [%s]> ", repoTitle, current)

	branches, err := deps.Git.AllBranches(deps.Ctx, repoPath)
	if err != nil {
		return "", fmt.Errorf("unable to read branches: %w", err)
	}

	sp := selection.New("", branches)
	sp.FilterPrompt = ps
	sp.FilterPlaceholder = "Select branch to checkout"
	sp.PageSize = 10
	sp.FinalChoiceStyle = func(choice *selection.Choice[string]) string {
		s := fmt.Sprintf("%s ", repoTitle)
		if choice.Value == current {
			return s + choice.Value
		}
		return s + deps.Theme.GitOutput.Render(
			fmt.Sprintf("Switched to branch %q", choice.Value),
		)
	}

	want, err := sp.RunPrompt()
	if err != nil {
		return "", err
	}
	if len(want) > 0 && want != current {
		return want, nil
	}
	return "", nil
}
