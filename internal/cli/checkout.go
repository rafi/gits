package cli

import (
	"fmt"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/pkg/git"

	"github.com/c-bata/go-prompt"
	log "github.com/sirupsen/logrus"
)

func Checkout(git git.Git, config Config, projects []domain.Project) error {
	for _, project := range projects {
		fmt.Print(project.GetTitle())

		for _, repoCfg := range project.Repos {
			repoPath, err := project.GetRepoAbsPath(repoCfg["dir"])
			if err != nil {
				return fmt.Errorf("Unable to get path: %w", err)
			}

			if !git.IsRepo(repoPath) {
				log.Warn(fmt.Sprintf("Not a Git repository %v\n", repoPath))
				continue
			}

			current, err := git.CurrentBranch(repoPath)
			if err != nil {
				return fmt.Errorf("Unable to get branch: %w", err)
			}
			ps := fmt.Sprintf("%v [%v]> ", repoCfg["dir"], current)

			want := prompt.Input(ps, BranchCompleter(git, repoPath))
			if len(want) > 0 {
				args := []string{"checkout", want}
				if _, err := git.Exec(repoPath, args); err != nil {
					return fmt.Errorf("Error during checkout: %w", err)
				}
			}
		}
	}

	return nil
}

// BranchCompleter use go-prompt to display list of branches with
// auto-completion.
func BranchCompleter(git git.Git, repoPath string) func(d prompt.Document) []prompt.Suggest {
	s := []prompt.Suggest{}
	branches, err := git.Branches(repoPath)
	if err != nil {
		log.Fatal(fmt.Sprintf("Unable to read branches: %s", err))
	}
	for _, branch := range branches {
		entry := prompt.Suggest{Text: branch}
		s = append(s, entry)
	}
	return func(d prompt.Document) []prompt.Suggest {
		return prompt.FilterHasPrefix(s, d.GetWordBeforeCursor(), true)
	}
}
