package cmd

import (
	"github.com/rafi/gits/internal/cli"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(fetchCmd)
}

// nolint:gochecknoglobals
var fetchCmd = &cobra.Command{
	Use:   "fetch <project>...",
	Short: "Fetches and prunes from all remotes",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		git := loadGit()
		projects := loadProjectsFromArgs(git, args)
		return cli.Fetch(git, cfg, projects)
	},
}
