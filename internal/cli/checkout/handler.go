package checkout

import (
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

	// Checkout all project's repositories.
	if repo == nil {
		var errList []cli.Error
		checkoutProjectRepos(project, deps, &errList)
		cli.HandlerErrors(errList)
		return nil
	}

	// Checkout a single repository.
	return checkoutRepo(project, *repo, deps)
}

func checkoutProjectRepos(project domain.Project, deps types.RuntimeCLI, errList *[]cli.Error) {
	errorStyle := deps.Theme.Error.Copy()

	fmt.Println(cli.ProjectTitleWithBullet(project, deps.Theme))

	for _, repo := range project.Repos {
		err := checkoutRepo(project, repo, deps)
		if err != nil {
			*errList = append(*errList, cli.Error{
				Message: fmt.Sprint(err),
				Title:   repo.GetName(),
				Dir:     repo.AbsPath,
			})
			fmt.Println(errorStyle.Render(err.Error()))
		}
	}

	for _, subProject := range project.SubProjects {
		fmt.Println()
		checkoutProjectRepos(subProject, deps, errList)
	}
}

func checkoutRepo(project domain.Project, repo domain.Repository, deps types.RuntimeCLI) error {
	repoDir := cli.Path(repo.AbsPath, deps.HomeDir)
	repoTitle := cli.RepoTitle(project, repo, deps.HomeDir).
		Inherit(deps.Theme.RepoTitle).
		MarginLeft(cli.LeftMargin).MarginRight(cli.RightMargin).
		Render()

	// Abort if repository is not cloned or has errors.
	if repo.State != domain.RepoStateOK {
		return cli.AbortOnRepoState(repo, deps.Theme)
	}

	gitRepo, err := deps.Git.Open(repo.AbsPath)
	if err != nil {
		return err
	}

	branch, err := promptRepo(repoTitle, repoDir, gitRepo, deps)
	if err != nil {
		return err
	}
	if branch != "" {
		return gitRepo.Checkout(branch)
	}
	return nil
}

// promptRepo prompts the user to select a branch to checkout.
func promptRepo(repoTitle string, repoPath string, gitRepo git.Repository, deps types.RuntimeCLI) (string, error) {
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
		s := fmt.Sprintf("%s> ", repoPath)
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
