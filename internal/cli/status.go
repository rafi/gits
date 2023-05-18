package cli

import (
	"fmt"
	"strconv"
	"strings"

	aur "github.com/logrusorgru/aurora"
	homedir "github.com/mitchellh/go-homedir"
	log "github.com/sirupsen/logrus"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/pkg/git"
)

func Status(git git.Git, config Config, projects []domain.Project) error {
	var (
		count     int
		modified  string
		untracked string
	)

	// Find home directory
	home, err := homedir.Dir()
	if err != nil {
		log.Fatal("Unable to find home directory, ", err)
	}

	for _, project := range projects {
		fmt.Print(project.GetTitle())
		maxLen := project.GetMaxLen()

		for _, repoCfg := range project.Repos {
			path, err := project.GetRepoAbsPath(repoCfg["dir"])
			if err != nil {
				log.Fatal(err)
			}

			if !git.IsRepo(path) {
				fmt.Printf(
					"  %"+strconv.Itoa(maxLen)+"v %12s %s\n",
					aur.Gray(12, strings.Replace(repoCfg["dir"], home, "~", 1)),
					aur.Magenta("x"),
					aur.Cyan("Not a Git repository"),
				)
				continue
			}
			version, err := git.Describe(path)
			if err != nil {
				version = ""
			}

			modified = ""
			if count, err = git.Modified(path); err != nil {
				return fmt.Errorf("Error in modified query: %w", err)
			} else if count > 0 {
				modified = fmt.Sprintf("≠%d", count)
			}
			untracked = ""
			if count, err = git.Untracked(path); err != nil {
				return fmt.Errorf("Error in untracked query: %w", err)
			} else if count > 0 {
				untracked = fmt.Sprintf("?%d", count)
			}

			leftMargin := 2
			diff, err := git.Diff(path)
			if err != nil {
				diff = "-"
			}
			currentRef, err := git.CurrentPosition(path)
			if err != nil {
				currentRef = "N/A"
			}

			fmt.Printf("%"+strconv.Itoa(maxLen+leftMargin)+"v %3v %3v %4v %v %v\n",
				aur.Gray(12, strings.Replace(repoCfg["dir"], home, "~", 1)),
				aur.Red(modified),
				aur.Blue(untracked),
				aur.Magenta(diff),
				currentRef,
				version,
			)
		}
	}
	return nil
}
