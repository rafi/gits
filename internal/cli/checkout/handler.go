package checkout

import (
	"errors"
	"fmt"

	"github.com/erikgeiser/promptkit/selection"
	log "github.com/sirupsen/logrus"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cli"
	"github.com/rafi/gits/internal/types"
	"github.com/rafi/gits/pkg/git"
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
		return checkoutRepo(project, *repo, deps)
	}

	// Checkout all project's repositories.
	errs := checkoutProjectRepos(project, deps)
	if len(errs) > 0 {
		cli.RenderErrors(errs, true)
		return errors.New("checkout completed with errors")
	}
	return nil
}

func checkoutProjectRepos(project domain.Project, deps types.RuntimeCLI) []error {
	fmt.Println(cli.ProjectTitleWithBullet(project, deps.Theme))

	errList := make([]error, 0)
	for _, repo := range project.Repos {
		err := checkoutRepo(project, repo, deps)
		if err != nil {
			errList = append(errList, err)
		}
	}

	for _, subProject := range project.SubProjects {
		fmt.Println()
		errs := checkoutProjectRepos(subProject, deps)
		errList = append(errList, errs...)
	}
	return errList
}

func checkoutRepo(project domain.Project, repo domain.Repository, deps types.RuntimeCLI) error {
	repoTitle := cli.RepoTitle(project, repo, deps.HomeDir, deps.Theme).
		Render()

	// Abort if repository is not cloned or has errors.
	if repo.State != domain.RepoStateOK {
		fmt.Print(repoTitle)
		defer fmt.Println()
		return cli.AbortOnRepoState(repo, deps.Theme.Error)
	}

	gitRepo, err := deps.Git.Open(repo.AbsPath)
	if err != nil {
		return err
	}

	branch, err := promptRepo(repoTitle, gitRepo, deps)
	if err != nil {
		return err
	}
	if branch == "" {
		return nil
	}

	err = gitRepo.Checkout(branch)
	if err != nil {
		fmt.Print(deps.Theme.Error.Render(err.Error()))
		return cli.RepoError(err, repo)
	}
	return nil
}

// promptRepo prompts the user to select a branch to checkout.
func promptRepo(repoTitle string, gitRepo git.Repository, deps types.RuntimeCLI) (string, error) {
	current, err := gitRepo.CurrentBranch()
	if err != nil {
		return "", fmt.Errorf("unable to get branch: %w", err)
	}

	ps := fmt.Sprintf("%s [%s]> ", repoTitle, current)

	branches, err := gitRepo.Branches()
	if err != nil {
		log.Fatal(fmt.Sprintf("Unable to read branches: %s", err))
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
