package cmd

import (
	"github.com/rafi/gits/internal/cli"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(cloneCmd)
}

// nolint:gochecknoglobals
var cloneCmd = &cobra.Command{
	Use:   "clone <project>...",
	Short: "Clones all repositories",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		git := loadGit()
		projects := loadProjectsFromArgs(git, args)
		return cli.Clone(git, cfg, projects)
	},
}
