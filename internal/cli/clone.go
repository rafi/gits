package cli

import (
	"fmt"
	"os"
	"strconv"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/pkg/git"

	aur "github.com/logrusorgru/aurora"
	log "github.com/sirupsen/logrus"
)

func Clone(git git.Git, config Config, projects []domain.Project) error {
	leftMargin := 2
	var color uint8 = 12

	for _, project := range projects {
		fmt.Print(project.GetTitle())
		maxLen := project.GetMaxLen()

		for _, repoCfg := range project.Repos {
			path, err := project.GetRepoAbsPath(repoCfg["dir"])
			if err != nil {
				return fmt.Errorf("Unable to get path: %w", err)
			}

			if _, err := os.Stat(path); !os.IsNotExist(err) {
				log.Warn(fmt.Sprintf("Directory already exists %v\n", path))
				continue
			}

			fmt.Printf(
				"%"+strconv.Itoa(maxLen+leftMargin)+"v ",
				aur.Gray(color, repoCfg["dir"]))

			if repoCfg["src"] != "" {
				if result, err := git.Clone(repoCfg["src"], path); err != nil {
					return fmt.Errorf("Unable to clone: %w", err)
				} else if result != "" {
					fmt.Println(result)
				}
			} else {
				fmt.Println("Missing 'src' attribute set for remote address")
			}
		}
	}

	return nil
}
