package cmd

import (
	"fmt"
	"github.com/logrusorgru/aurora"
	homedir "github.com/mitchellh/go-homedir"
	"github.com/rafi/gits/common"
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
				if len(projectBasePath) > 0 {
					path = filepath.Join(projectBasePath, path)
				}

				args = []string{"fetch", "--all", "--tags", "--prune"}
				output := common.GitRun(path, args, true)

				fmt.Printf("  %v %v\n",
					aurora.Gray(repoCfg["dir"]),
					strings.TrimSuffix(string(output), "\n"),
				)
			}
		}
	},
}
