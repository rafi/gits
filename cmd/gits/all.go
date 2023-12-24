package main

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/rafi/gits/internal/cli/browse"
	"github.com/rafi/gits/internal/cli/checkout"
	"github.com/rafi/gits/internal/cli/clone"
	"github.com/rafi/gits/internal/cli/fetch"
	"github.com/rafi/gits/internal/cli/list"
	"github.com/rafi/gits/internal/cli/status"
	"github.com/rafi/gits/internal/cli/sync"
	"github.com/rafi/gits/internal/cli/types"
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

	// Gits commands.
	rootCmd.AddCommand(branchOverviewCmd)
	rootCmd.AddCommand(browseCmd)
	rootCmd.AddCommand(checkoutCmd)
	rootCmd.AddCommand(cloneCmd)
	rootCmd.AddCommand(fetchCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(repoOverviewCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(syncCmd)
	rootCmd.AddCommand(versionCmd)
}

var branchOverviewCmd = &cobra.Command{
	Use:               "branch-overview <project> <repo> [branch]",
	Args:              cobra.RangeArgs(2, 3),
	ValidArgsFunction: completeProjectRepoBranch,
	RunE:              runWithDeps(browse.ExecBranchOverview),
	Hidden:            true,
}

var browseCmd = &cobra.Command{
	Use:               "browse [project] [repo] [branch]",
	Short:             "Browse branches and tags",
	Args:              cobra.MaximumNArgs(3),
	ValidArgsFunction: completeProjectRepoBranch,
	RunE:              runWithDeps(browse.ExecBrowse),
}

var checkoutCmd = &cobra.Command{
	Use:               "checkout [project] [repo]",
	Short:             "Checkout branch from multiple repositories, or single",
	Args:              cobra.MaximumNArgs(2),
	ValidArgsFunction: completeProjectRepo,
	RunE:              runWithDeps(checkout.ExecCheckout),
}

var cloneCmd = &cobra.Command{
	Use:               "clone [project] [repo]",
	Short:             "Clone all repositories",
	Args:              cobra.MaximumNArgs(2),
	ValidArgsFunction: completeProjectRepo,
	RunE:              runWithDeps(clone.ExecClone),
}

var fetchCmd = &cobra.Command{
	Use:               "fetch [project] [repo]",
	Short:             "Fetch and prune from all remotes",
	Args:              cobra.MaximumNArgs(2),
	ValidArgsFunction: completeProjectRepo,
	RunE:              runWithDeps(fetch.ExecFetch),
}

var listCmd = &cobra.Command{
	Use:               "list [project]...",
	Short:             "List all projects or their repositories",
	Aliases:           []string{"ls"},
	Args:              cobra.ArbitraryArgs,
	ValidArgsFunction: completeProject,
	RunE: runWithDeps(func(args []string, deps types.RuntimeDeps) error {
		// Run with output style.
		return list.ExecList(listOutput, args, deps)
	}),
}

var repoOverviewCmd = &cobra.Command{
	Use:               "repo-overview <project> <repo>", // TODO: make optional
	Args:              cobra.ExactArgs(2),
	ValidArgsFunction: completeProjectRepoBranch,
	RunE:              runWithDeps(browse.ExecRepoOverview),
	Hidden:            true,
}

var statusCmd = &cobra.Command{
	Use:               "status <project>...",
	Short:             "Show Git repositories short status",
	Args:              cobra.MinimumNArgs(1),
	ValidArgsFunction: completeProject,
	RunE:              runWithDeps(status.ExecStatus),
}

var syncCmd = &cobra.Command{
	Use:               "sync [project]...",
	Short:             "Synchronize project indexes",
	Args:              cobra.ArbitraryArgs,
	ValidArgsFunction: completeProject,
	RunE:              runWithDeps(sync.ExecSync),
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show version",
	Args:  cobra.NoArgs,
	Run: func(_ *cobra.Command, _ []string) {
		fmt.Printf("gits %s %s\n", version.GetVersion(), runtime.Version())
	},
}
