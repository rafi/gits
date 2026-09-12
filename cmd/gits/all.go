// Package main implements the gits command: a fast CLI Git manager for
// multiple repositories grouped by projects, with GitHub, GitLab and
// Bitbucket support.
package main

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/rafi/gits/internal/cli/add"
	"github.com/rafi/gits/internal/cli/browse"
	"github.com/rafi/gits/internal/cli/cd"
	"github.com/rafi/gits/internal/cli/checkout"
	"github.com/rafi/gits/internal/cli/clone"
	"github.com/rafi/gits/internal/cli/fetch"
	"github.com/rafi/gits/internal/cli/list"
	"github.com/rafi/gits/internal/cli/orphan"
	"github.com/rafi/gits/internal/cli/pull"
	"github.com/rafi/gits/internal/cli/push"
	"github.com/rafi/gits/internal/cli/status"
	"github.com/rafi/gits/internal/cli/sync"
	"github.com/rafi/gits/internal/git"
	"github.com/rafi/gits/internal/types"
	"github.com/rafi/gits/internal/version"
)

const (
	appName  = "gits"
	appShort = "gits is a tool for managing multiple Git repositories"
	appLong  = `Fast CLI Git manager for multiple repositories grouped by projects, with GitHub/GitLab/Bitbucket support.`
)

var listOutput = "table"

var statusOutput = "table"

var statusOpts status.Options

// pushOpts is the vetted passthrough set from docs/adr/0002-push-safety-model.md.
// Nothing here reaches --force, -u or --mirror, and nothing should be added
// that does.
var pushOpts git.PushOptions

func init() {
	listCmd.
		PersistentFlags().
		StringVarP(&listOutput, "output", "o", listOutput, "output style (json, name, table, tree, wide)")

	statusCmd.
		PersistentFlags().
		StringVarP(&statusOutput, "output", "o", statusOutput, "output style (json, table)")
	statusCmd.
		PersistentFlags().
		BoolVar(&statusOpts.Stat, "stat", false, "show HEAD± column with uncommitted line diffs")
	statusCmd.
		PersistentFlags().
		BoolVar(&statusOpts.Dirty, "dirty", false, "show only repos with uncommitted changes")
	statusCmd.
		PersistentFlags().
		BoolVar(&statusOpts.Unsynced, "unsynced", false, "show only repos ahead or behind upstream")

	pushFlags := pushCmd.PersistentFlags()
	pushFlags.BoolVar(&pushOpts.All, "all", false, "push all branches")
	pushFlags.BoolVar(&pushOpts.Branches, "branches", false, "push all branches (synonym of --all)")
	pushFlags.BoolVar(&pushOpts.Tags, "tags", false, "push all tags instead of the current branch")
	pushFlags.BoolVar(&pushOpts.FollowTags, "follow-tags", false, "also push reachable annotated tags")
	pushFlags.BoolVar(&pushOpts.Atomic, "atomic", false, "push all refs in one remote transaction")
	pushFlags.BoolVar(&pushOpts.Prune, "prune", false, "remove remote refs matching the pushed refspec")
	pushFlags.BoolVarP(&pushOpts.DryRun, "dry-run", "n", false, "report what would be pushed, push nothing")

	rootCmd.AddCommand(addCmd)
	rootCmd.AddCommand(branchOverviewCmd)
	rootCmd.AddCommand(browseCmd)
	rootCmd.AddCommand(cdCmd)
	rootCmd.AddCommand(checkoutCmd)
	rootCmd.AddCommand(cloneCmd)
	rootCmd.AddCommand(fetchCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(orphanCmd)
	rootCmd.AddCommand(pullCmd)
	rootCmd.AddCommand(pushCmd)
	rootCmd.AddCommand(repoOverviewCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(syncCmd)
	rootCmd.AddCommand(versionCmd)
}

var addCmd = &cobra.Command{
	Use:               "add [project] [repo]",
	Short:             "Add repository to a project",
	Args:              cobra.MaximumNArgs(2),
	ValidArgsFunction: completeProject,
	RunE:              runWithDeps(add.ExecAdd),
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
	Short:             "Traverse repositories and optionally checkout branch",
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
	Short:             "List project repositories",
	Aliases:           []string{"ls"},
	Args:              cobra.ArbitraryArgs,
	ValidArgsFunction: completeProject,
	RunE: runWithDeps(func(args []string, deps types.RuntimeCLI) error {
		// Run with output style.
		return list.ExecList(listOutput, args, deps)
	}),
}

var orphanCmd = &cobra.Command{
	Use:               "orphan [project] [repo]",
	Short:             "Finds orphan repository",
	Args:              cobra.MaximumNArgs(2),
	ValidArgsFunction: completeProjectRepo,
	RunE:              runWithDeps(orphan.ExecOrphan),
}

var pullCmd = &cobra.Command{
	Use:               "pull [project] [repo]",
	Short:             "Pull repository",
	Args:              cobra.MaximumNArgs(2),
	ValidArgsFunction: completeProjectRepo,
	RunE:              runWithDeps(pull.ExecPull),
}

var pushCmd = &cobra.Command{
	Use:               "push [project] [repo]",
	Short:             "Push current branch to its upstream",
	Args:              cobra.MaximumNArgs(2),
	ValidArgsFunction: completeProjectRepo,
	RunE: runWithDeps(func(args []string, deps types.RuntimeCLI) error {
		return push.ExecPush(pushOpts, args, deps)
	}),
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
	RunE: runWithDeps(func(args []string, deps types.RuntimeCLI) error {
		return status.ExecStatus(statusOutput, statusOpts, args, deps)
	}),
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
