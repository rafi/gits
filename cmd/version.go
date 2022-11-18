package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// version will contain version on build
var version string

func init() {
	rootCmd.AddCommand(versionCmd)
}

// nolint:gochecknoglobals
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show version",
	Args:  cobra.ExactArgs(0),
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("gits: %v\n", version)
	},
}
