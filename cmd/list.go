package cmd

import (
	"github.com/rafi/gits/internal/cli"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(listCmd)
}

// nolint:gochecknoglobals
var listCmd = &cobra.Command{
	Use:   "list [project]...",
	Short: "Lists all projects or their repositories",
	Args:  cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		git := loadGit()
		projects := loadProjectsFromArgs(git, args)
		return cli.List(git, cfg, projects)
	},
}
