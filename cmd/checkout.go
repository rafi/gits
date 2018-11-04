package cmd

import (
	"fmt"
	"github.com/c-bata/go-prompt"
	"github.com/logrusorgru/aurora"
	homedir "github.com/mitchellh/go-homedir"
	"github.com/rafi/gits/common"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"path/filepath"
	"strings"
)

var repoPath string

func init() {
	rootCmd.AddCommand(checkoutCmd)
}

var checkoutCmd = &cobra.Command{
	Use:   "checkout <project>",
	Short: "Traverse repositories and optionally checkout branch",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		for _, projectName := range args {
			fmt.Printf("%v %v\n", aurora.Blue("::"), projectName)
			project := cfg.Projects[projectName]
			projectBasePath, err := homedir.Expand(project.Path)
			if err != nil {
				log.Fatal(err)
			}

			for _, repoCfg := range project.Repos {
				path, err := homedir.Expand(repoCfg["dir"])
				if err != nil {
					log.Fatal(err)
				}
				if len(projectBasePath) > 0 && string(path[0]) != "/" {
					path = filepath.Join(projectBasePath, path)
				}
				repoPath = path

				current := GitCurrentBranch(path)
				ps := fmt.Sprintf("%v [%v]> ", repoCfg["dir"], current)

				want := prompt.Input(ps, BranchCompleter)
				if len(want) > 0 {
					args := []string{"checkout", want}
					common.GitRun(path, args, true)
				}
			}
		}
	},
}

// BranchCompleter use go-prompt to display list
// of branches with auto-completion
func BranchCompleter(d prompt.Document) []prompt.Suggest {
	s := []prompt.Suggest{}
	for _, branch := range GitBranches(repoPath) {
		entry := prompt.Suggest{Text: branch}
		s = append(s, entry)
	}
	return prompt.FilterHasPrefix(s, d.GetWordBeforeCursor(), true)
}

// GitCurrentBranch returns current branch
func GitCurrentBranch(path string) string {
	args := []string{"rev-parse", "--abbrev-ref", "HEAD"}
	output := common.GitRun(path, args, true)
	branch := strings.TrimSuffix(string(output), "\n")
	return branch
}

// GitBranches returns list of branches, local and remote
func GitBranches(path string) []string {
	args := []string{"for-each-ref", "--shell", "--format=%(refname)", "refs"}
	output := common.GitRun(path, args, true)
	refs := strings.Split(strings.TrimSuffix(string(output), "\n"), "\n")
	branches := []string{}
	for _, ref := range refs {
		ref = strings.Trim(ref, "'")
		parts := strings.Split(ref, "/")
		if parts[len(parts)-1] != "HEAD" {
			ref := strings.Join(parts[2:], "/")
			if len(ref) > 0 {
				branches = append(branches, ref)
			}
		}
	}
	return branches
}
