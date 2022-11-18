package cli

import (
	"fmt"
	"strings"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/pkg/git"

	aur "github.com/logrusorgru/aurora"
	log "github.com/sirupsen/logrus"
)

func Fetch(git git.Git, config Config, projects []domain.Project) error {
	var color uint8 = 12
	for _, project := range projects {
		fmt.Print(project.GetTitle())

		for _, repoCfg := range project.Repos {
			path, err := project.GetRepoAbsPath(repoCfg["dir"])
			if err != nil {
				return fmt.Errorf("Unable to get path: %w", err)
			}

			if !git.IsRepo(path) {
				log.Warn(fmt.Sprintf("Not a Git repository %v\n", path))
				continue
			}

			args := []string{"fetch", "--all", "--tags", "--prune", "--force"}
			output, err := git.Exec(path, args)
			if err != nil {
				return fmt.Errorf("Error during fetch: %w", err)
			}

			fmt.Printf("  %v %v\n",
				aur.Gray(color, repoCfg["dir"]),
				strings.TrimSuffix(string(output), "\n"),
			)
		}
	}

	return nil
}
