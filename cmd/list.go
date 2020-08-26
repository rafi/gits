package cmd

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/rafi/gits/common"

	aur "github.com/logrusorgru/aurora"
	homedir "github.com/mitchellh/go-homedir"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(listCmd)
}

var listCmd = &cobra.Command{
	Use:   "list [project]...",
	Short: "Lists all projects or their repositories",
	Args:  cobra.ArbitraryArgs,
	Run: func(cmd *cobra.Command, args []string) {

		if len(args) == 0 {
			for projectName := range cfg.Projects {
				fmt.Println(projectName)
			}

		} else {

			// Find home directory
			home, err := homedir.Dir()
			if err != nil {
				log.Fatal("Unable to find home directory, ", err)
			}

			for _, projectName := range args {
				project, err := cfg.GetProject(projectName)
				if err != nil {
					log.Fatal(err)
				}

				fmt.Println(project.Name)
				maxLen := project.GetMaxLen()

				for _, repoCfg := range project.Repos {
					path, err := project.GetRepoAbsPath(repoCfg["dir"])
					if err != nil {
						log.Fatal(err)
					}

					var state aur.Value
					if _, err := os.Stat(path); os.IsNotExist(err) {
						state = aur.Magenta("Doesn't exist")
					} else if !common.GitIsRepo(path) {
						state = aur.Magenta("Not a Git repository")
					}

					fmt.Printf(
						"  %-"+strconv.Itoa(maxLen+2)+"v",
						aur.Gray(12, strings.Replace(repoCfg["dir"], home, "~", 1)),
					)
					if state != nil {
						fmt.Printf(" (%v)", state)
					}
					fmt.Println()
				}
			}
		}
	},
}
