package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"

	aur "github.com/logrusorgru/aurora"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(cloneCmd)
}

var cloneCmd = &cobra.Command{
	Use:   "clone <project>...",
	Short: "Clones all repositories",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
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

				if _, err := os.Stat(path); !os.IsNotExist(err) {
					log.Warn(fmt.Sprintf("Directory already exists %v\n", path))
					continue
				}

				fmt.Printf(
					"%"+strconv.Itoa(maxLen+2)+"v ",
					aur.Gray(12, repoCfg["dir"]))

				if repoCfg["src"] != "" {
					result := GitClone(repoCfg["src"], path)
					if result != "" {
						fmt.Println(result)
					}
				} else {
					fmt.Println("Missing 'src' attribute set for remote address")
				}
			}
		}
	},
}

// GitClone clones repository, if not cloned already
func GitClone(remote string, path string) string {
	var output []byte
	_, err := os.Stat(path)
	if err != nil && os.IsNotExist(err) {
		args := []string{"clone", remote, path}
		cmd := exec.Command("git", args...)
		if output, err = cmd.CombinedOutput(); err != nil {
			log.Fatal(err)
		}
	} else {
		fmt.Println("Path already exists")
	}
	return string(output)
}
