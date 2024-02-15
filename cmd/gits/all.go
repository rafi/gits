package main

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/rafi/gits/internal/cli/browse"
	"github.com/rafi/gits/internal/cli/cd"
	"github.com/rafi/gits/internal/cli/checkout"
	"github.com/rafi/gits/internal/cli/clone"
	"github.com/rafi/gits/internal/cli/fetch"
	"github.com/rafi/gits/internal/cli/list"
	"github.com/rafi/gits/internal/cli/pull"
	"github.com/rafi/gits/internal/cli/status"
	"github.com/rafi/gits/internal/cli/sync"
	"github.com/rafi/gits/internal/types"
	"github.com/rafi/gits/internal/version"
)

const (
	appName  = "gits"
	appShort = "gits is a tool for managing multiple Git repositories"
	appLong  = `Fast CLI Git manager for multiple repositories with GitHub/GitLab/Bitbucket support`
)

var listOutput = "table"

func init() {
	listCmd.
		PersistentFlags().
		StringVarP(&listOutput, "output", "o", listOutput, "output style (json, name, table, tree, wide)")

	rootCmd.AddCommand(branchOverviewCmd)
	rootCmd.AddCommand(browseCmd)
	rootCmd.AddCommand(cdCmd)
	rootCmd.AddCommand(checkoutCmd)
	rootCmd.AddCommand(cloneCmd)
	rootCmd.AddCommand(fetchCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(pullCmd)
	rootCmd.AddCommand(repoOverviewCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(syncCmd)
	rootCmd.AddCommand(versionCmd)
}

var branchOverviewCmd = &cobra.Command{
	Use:               "branch-overview <project> <repo> [branch]",
	Hidden:            true,
	Args:              cobra.RangeArgs(2, 3),
	ValidArgsFunction: completeProjectRepoBranch,
	RunE:              runWithDeps(browse.ExecBranchOverview),
}

var browseCmd = &cobra.Command{
	Use:               "browse [project] [repo] [branch]",
	Short:             "Browse branches and tags",
	Args:              cobra.MaximumNArgs(3),
	ValidArgsFunction: completeProjectRepoBranch,
	RunE:              runWithDeps(browse.ExecBrowse),
}

var cdCmd = &cobra.Command{
	Use:               "cd [project] [repo]",
	Short:             "Get repository path",
	Args:              cobra.MaximumNArgs(2),
	ValidArgsFunction: completeProjectRepo,
	RunE:              runWithDeps(cd.ExecCD),
}

var checkoutCmd = &cobra.Command{
	Use:               "checkout [project] [repo]",
	Short:             "Checkout branch from multiple repositories, or single",
	Aliases:           []string{"ck"},
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
	RunE: runWithDeps(func(args []string, deps types.RuntimeCLI) error {
		// Run with output style.
		return list.ExecList(listOutput, args, deps)
	}),
}

var pullCmd = &cobra.Command{
	Use:               "pull [project] [repo]",
	Short:             "Pull repository",
	Args:              cobra.MaximumNArgs(2),
	ValidArgsFunction: completeProjectRepo,
	RunE:              runWithDeps(pull.ExecPull),
}

var repoOverviewCmd = &cobra.Command{
	Use:               "repo-overview <project> <repo>",
	Hidden:            true,
	Args:              cobra.ExactArgs(2),
	ValidArgsFunction: completeProjectRepo,
	RunE:              runWithDeps(browse.ExecRepoOverview),
}

var statusCmd = &cobra.Command{
	Use:               "status [project] [repo]",
	Short:             "Show Git repositories short status",
	Args:              cobra.MaximumNArgs(2),
	ValidArgsFunction: completeProjectRepo,
	RunE:              runWithDeps(status.ExecStatus),
}

var syncCmd = &cobra.Command{
	Use:               "sync [project]...",
	Short:             "Synchronize project caches",
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
