package cmd

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	// "github.com/davecgh/go-spew/spew"
	. "github.com/logrusorgru/aurora"
	homedir "github.com/mitchellh/go-homedir"
	"github.com/rafi/gmux/common"
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

		for _, project_name := range args {
			fmt.Printf("%v %v\n", Blue("::"), project_name)
			project := common.GetProject(project_name)
			project_base_path, err := homedir.Expand(project.Path)
			if err != nil {
				log.Fatal(err)
			}

			max_len := 0
			for _, repo_cfg := range project.Repos {
				if i := len(repo_cfg["dir"]); i > max_len {
					max_len = i
				}
			}

			for _, repo_cfg := range project.Repos {
				path, err := homedir.Expand(repo_cfg["dir"])
				if err != nil {
					log.Fatal(err)
				}
				if len(project_base_path) > 0 && string(path[0]) != "/" {
					path = filepath.Join(project_base_path, path)
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

				fmt.Printf("%"+strconv.Itoa(max_len+2)+"v %3v %3v %4v %v %v\n",
					Gray(repo_cfg["dir"]),
					Red(modified),
					Blue(untracked),
					Magenta(GitDiff(path)),
					GitCurrentPosition(path),
					version,
				)
			}
		}
	},
}

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

func GitUntracked(path string) int {
	args := []string{"ls-files", "--others", "--exclude-standard"}
	output := common.GitRun(path, args, true)
	return len(strings.Split(string(output), "\n")) - 1
}

func GitCurrentPosition(path string) string {
	args := []string{"log", "-1", "--color=always", "--format=%C(auto)%D %C(242)(%aN %ar)%Creset"}
	output := common.GitRun(path, args, true)
	return strings.TrimSuffix(string(output), "\n")
}

func GitDescribe(path string) string {
	args := []string{"describe", "--always"}
	return strings.TrimSuffix(string(common.GitRun(path, args, true)), "\n")
}

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
