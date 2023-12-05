package main

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/rafi/gits/internal/cli"
	"github.com/rafi/gits/internal/cli/checkout"
	"github.com/rafi/gits/internal/cli/clone"
	"github.com/rafi/gits/internal/cli/fetch"
	"github.com/rafi/gits/internal/cli/list"
	"github.com/rafi/gits/internal/cli/status"
	"github.com/rafi/gits/internal/cli/sync"
	"github.com/rafi/gits/internal/version"
)

const (
	appName  = "gits"
	appShort = "gits is a tool for managing multiple Git repositories"
	appLong  = `Fast CLI Git manager for multiple repositories with cloud support`
)

var listOutput = "table"

func init() {
	listCmd.
		PersistentFlags().
		StringVarP(&listOutput, "output", "o", listOutput, "output style (json, name, table, tree, wide)")

	rootCmd.AddCommand(checkoutCmd)
	rootCmd.AddCommand(cloneCmd)
	rootCmd.AddCommand(fetchCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(syncCmd)
	rootCmd.AddCommand(versionCmd)
}

// nolint:gochecknoglobals
var checkoutCmd = &cobra.Command{
	Use:               "checkout <project>...",
	Short:             "Traverse repositories and optionally checkout branch",
	Args:              cobra.MinimumNArgs(1),
	ValidArgsFunction: completeProjectNames,
	RunE:              runWithDeps(checkout.ExecCheckout),
}

// nolint:gochecknoglobals
var cloneCmd = &cobra.Command{
	Use:               "clone <project>...",
	Short:             "Clone all repositories",
	Args:              cobra.MinimumNArgs(1),
	ValidArgsFunction: completeProjectNames,
	RunE:              runWithDeps(clone.ExecClone),
}

// nolint:gochecknoglobals
var fetchCmd = &cobra.Command{
	Use:               "fetch <project>...",
	Short:             "Fetch and prune from all remotes",
	Args:              cobra.MinimumNArgs(1),
	ValidArgsFunction: completeProjectNames,
	RunE:              runWithDeps(fetch.ExecFetch),
}

// nolint:gochecknoglobals
var listCmd = &cobra.Command{
	Use:               "list [project]...",
	Short:             "List all projects or their repositories",
	Args:              cobra.ArbitraryArgs,
	ValidArgsFunction: completeProjectNames,
	RunE: runWithDeps(func(args []string, deps cli.RuntimeDeps) error {
		return list.ExecList(listOutput, args, deps)
	}),
}

// nolint:gochecknoglobals
var statusCmd = &cobra.Command{
	Use:               "status <project>...",
	Short:             "Show Git repositories short status",
	Args:              cobra.MinimumNArgs(1),
	ValidArgsFunction: completeProjectNames,
	RunE:              runWithDeps(status.ExecStatus),
}

// nolint:gochecknoglobals
var syncCmd = &cobra.Command{
	Use:               "sync <project>...",
	Short:             "Synchronize project indexes",
	Args:              cobra.ArbitraryArgs,
	ValidArgsFunction: completeProjectNames,
	RunE:              runWithDeps(sync.ExecSync),
}

// nolint:gochecknoglobals
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show version",
	Args:  cobra.ExactArgs(0),
	Run: func(_ *cobra.Command, _ []string) {
		fmt.Printf("gits %s %s\n", version.GetVersion(), runtime.Version())
	},
}

// completeProjectNames returns a list of project names for shell completion.
func completeProjectNames(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	var completions []string
	for key, proj := range configFile.Projects {
		if toComplete == "" || strings.HasPrefix(key, toComplete) {
			completions = append(completions, fmt.Sprintf("%s\t%s", key, proj.Desc))
		}
	}
	return completions, cobra.ShellCompDirectiveNoFileComp
}
