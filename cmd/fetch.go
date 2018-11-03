package cmd

import (
	"fmt"
	. "github.com/logrusorgru/aurora"
	homedir "github.com/mitchellh/go-homedir"
	"github.com/rafi/gmux/common"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"path/filepath"
	"strings"
)

func init() {
	rootCmd.AddCommand(fetchCmd)
}

var fetchCmd = &cobra.Command{
	Use:   "fetch <project>",
	Short: "Fetches and prunes from all remotes",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {

		for _, project_name := range args {
			fmt.Printf("%v %v\n", Blue("::"), project_name)
			project := common.GetProject(project_name)
			project_base_path, err := homedir.Expand(project.Path)
			if err != nil {
				log.Fatal(err)
			}

			for _, repo_cfg := range project.Repos {
				path, err := homedir.Expand(repo_cfg["dir"])
				if err != nil {
					log.Fatal(err)
				}
				if len(project_base_path) > 0 {
					path = filepath.Join(project_base_path, path)
				}

				args = []string{"fetch", "--all", "--tags", "--prune"}
				output := common.GitRun(path, args, true)

				fmt.Printf("  %v %v\n",
					Gray(repo_cfg["dir"]),
					strings.TrimSuffix(string(output), "\n"),
				)
			}
		}
	},
}
