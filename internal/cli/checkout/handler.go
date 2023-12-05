package checkout

import (
	"fmt"

	"github.com/erikgeiser/promptkit/selection"
	log "github.com/sirupsen/logrus"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cli"
	"github.com/rafi/gits/internal/project"
	"github.com/rafi/gits/pkg/git"
)

func ExecCheckout(include []string, deps cli.RuntimeDeps) error {
	projects, err := project.GetProjects(include, deps)
	if err != nil {
		return fmt.Errorf("unable to list projects: %w", err)
	}
	errorStyle := deps.Theme.Error.Copy()

	for _, project := range projects {
		fmt.Println(cli.ProjectTitle(project, deps.Theme))
		for _, repo := range project.Repos {
			repoDir := cli.Path(repo.AbsPath, deps.HomeDir)
			repoTitle := cli.RepoTitle(project, repo, deps.HomeDir).
				Inherit(deps.Theme.RepoTitle).
				MarginLeft(cli.LeftMargin).MarginRight(cli.RightMargin).
				Render()

			switch repo.State {
			case domain.RepoStateError:
				fmt.Printf(" %s> %s\n", repoDir, errorStyle.Render("Not a Git repository"))
				continue
			case domain.RepoStateNoLocal:
				fmt.Printf(" %s> %s\n", repoDir, errorStyle.Render("Not cloned"))
				continue
			}

			gitRepo, err := deps.Git.Open(repo.AbsPath)
			if err != nil {
				return err
			}

			branch, err := promptRepo(repoTitle, repoDir, gitRepo, deps)
			if err != nil {
				log.Warn(err)
				break
			}
			if branch != "" {
				err := gitRepo.Checkout(branch)
				if err != nil {
					return fmt.Errorf("error during checkout: %w", err)
				}
			}
		}
	}
	return nil
}

func promptRepo(repoTitle string, repoPath string, gitRepo git.Repository, deps cli.RuntimeDeps) (string, error) {
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
