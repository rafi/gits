package cmd

import (
	"github.com/rafi/gits/internal/cli"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(statusCmd)
}

// nolint:gochecknoglobals
var statusCmd = &cobra.Command{
	Use:   "status <project>...",
	Short: "Shows Git repositories short status",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		git := loadGit()
		projects := loadProjectsFromArgs(git, args)
		return cli.Status(git, cfg, projects)
	},
}
