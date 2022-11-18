package cli

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/pkg/git"

	aur "github.com/logrusorgru/aurora"
	"github.com/mitchellh/go-homedir"
)

func List(git git.Git, config Config, projects []domain.Project) error {
	if len(projects) == 0 {
		for projectName := range config.Projects {
			fmt.Println(projectName)
		}
	} else {

		// Find home directory
		home, err := homedir.Dir()
		if err != nil {
			return fmt.Errorf("Unable to find home directory: %w", err)
		}

		leftMargin := 2
		var color uint8 = 12

		for _, project := range projects {
			fmt.Println(project.Name)
			maxLen := project.GetMaxLen()

			for _, repoCfg := range project.Repos {
				path, err := project.GetRepoAbsPath(repoCfg["dir"])
				if err != nil {
					return fmt.Errorf("Unable to get path: %w", err)
				}

				var state aur.Value
				if _, err := os.Stat(path); os.IsNotExist(err) {
					state = aur.Magenta("Doesn't exist")
				} else if !git.IsRepo(path) {
					state = aur.Magenta("Not a Git repository")
				}

				fmt.Printf(
					"  %-"+strconv.Itoa(maxLen+leftMargin)+"v",
					aur.Gray(color, strings.Replace(repoCfg["dir"], home, "~", 1)),
				)
				if state != nil {
					fmt.Printf(" (%v)", state)
				}
				fmt.Println()
			}
		}
	}

	return nil
}
