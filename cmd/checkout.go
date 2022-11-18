package cmd

import (
	"github.com/rafi/gits/internal/cli"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(checkoutCmd)
}

// nolint:gochecknoglobals
var checkoutCmd = &cobra.Command{
	Use:   "checkout <project>...",
	Short: "Traverse repositories and optionally checkout branch",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		git := loadGit()
		projects := loadProjectsFromArgs(git, args)
		return cli.Checkout(git, cfg, projects)
	},
}
