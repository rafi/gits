package cmd

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	// "github.com/davecgh/go-spew/spew"
	aur "github.com/logrusorgru/aurora"
	"github.com/rafi/gits/common"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(statusCmd)
}

var statusCmd = &cobra.Command{
	Use:   "status <project>",
	Short: "Shows Git repositories short status",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {

		var (
			count     int
			modified  string
			untracked string
		)

		for _, projectName := range args {
			project, err := cfg.GetProject(projectName)
			if err != nil {
				log.Fatal(err)
			}

			fmt.Print(project.GetTitle())
			maxLen := project.GetMaxLen()

			for _, repoCfg := range project.Repos {
				path, err := project.GetRepoAbsPath(repoCfg["dir"])
				if err != nil {
					log.Fatal(err)
				}

				version := GitDescribe(path)

				modified = ""
				if count = GitModified(path); count > 0 {
					modified = fmt.Sprintf("≠%d", count)
				}
				untracked = ""
				if count = GitUntracked(path); count > 0 {
					untracked = fmt.Sprintf("?%d", count)
				}

				fmt.Printf("%"+strconv.Itoa(maxLen+2)+"v %3v %3v %4v %v %v\n",
					aur.Gray(repoCfg["dir"]),
					aur.Red(modified),
					aur.Blue(untracked),
					aur.Magenta(GitDiff(path)),
					GitCurrentPosition(path),
					version,
				)
			}
		}
	},
}

// GitModified returns the number of modified files
func GitModified(path string) int {
	args := []string{"diff", "--shortstat"}
	output := common.GitRun(path, args, true)
	pat := regexp.MustCompile(`^\s*(\d+)`)
	matches := pat.FindAllStringSubmatch(string(output), -1)
	if len(matches) > 0 {
		modified, err := strconv.Atoi(matches[0][1])
		if err != nil {
			log.Fatal(err)
		}
		return modified
	}
	return 0
}

// GitUntracked returns the number of untracked files
func GitUntracked(path string) int {
	args := []string{"ls-files", "--others", "--exclude-standard"}
	output := common.GitRun(path, args, true)
	return len(strings.Split(string(output), "\n")) - 1
}

// GitCurrentPosition returns a short log description of HEAD
func GitCurrentPosition(path string) string {
	args := []string{"log", "-1", "--color=always", "--format=%C(auto)%D %C(242)(%aN %ar)%Creset"}
	output := common.GitRun(path, args, true)
	return strings.TrimSuffix(string(output), "\n")
}

// GitDescribe generates a version description based on tags and hash
func GitDescribe(path string) string {
	args := []string{"describe", "--always"}
	return strings.TrimSuffix(string(common.GitRun(path, args, true)), "\n")
}

// GitDiff returns a formatted string of ahead/behind counts
func GitDiff(path string) string {
	args := []string{"rev-parse", "--abbrev-ref", "HEAD"}
	branch := strings.TrimSuffix(string(common.GitRun(path, args, true)), "\n")

	args = []string{"rev-parse", "--abbrev-ref", "@{upstream}"}
	upstream := strings.TrimSuffix(string(common.GitRun(path, args, false)), "\n")
	if upstream == "" {
		upstream = fmt.Sprintf("origin/%v", branch)
	}

	args = []string{"rev-list", "--left-right", branch + "..." + upstream}
	output := common.GitRun(path, args, false)

	result := ""
	if len(output) == 0 {
		result = "✓"
	} else {
		behind := 0
		ahead := 0
		for _, rev := range strings.Split(string(output), "\n") {
			if rev == "" {
				continue
			}
			rev = string(rev[0])
			if rev == ">" {
				behind++
			}
			if rev == "<" {
				ahead++
			}
		}

		if ahead > 0 {
			result = fmt.Sprintf("▲%d", ahead)
		}
		if behind > 0 {
			result = fmt.Sprintf("%v▼%d", result, behind)
		}
	}

	return result
}
