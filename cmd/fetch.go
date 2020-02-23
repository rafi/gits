package cmd

import (
	"fmt"
	aur "github.com/logrusorgru/aurora"
	"github.com/rafi/gits/common"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
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
			project, err := cfg.GetProject(projectName)
			if err != nil {
				log.Fatal(err)
			}

			fmt.Print(project.GetTitle())

			for _, repoCfg := range project.Repos {
				path, err := project.GetRepoAbsPath(repoCfg["dir"])
				if err != nil {
					log.Fatal(err)
				}

				args = []string{"fetch", "--all", "--tags", "--prune"}
				output := common.GitRun(path, args, true)

				fmt.Printf("  %v %v\n",
					aur.Gray(12, repoCfg["dir"]),
					strings.TrimSuffix(string(output), "\n"),
				)
			}
		}
	},
}
