// Package main implements the gits command: a fast CLI Git manager for
// multiple repositories grouped by projects, with GitHub, GitLab and
// Bitbucket support.
package main

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/rafi/gits/internal/bulk"
	"github.com/rafi/gits/internal/cli/add"
	"github.com/rafi/gits/internal/cli/browse"
	"github.com/rafi/gits/internal/cli/cd"
	"github.com/rafi/gits/internal/cli/checkout"
	"github.com/rafi/gits/internal/cli/clone"
	"github.com/rafi/gits/internal/cli/doctor"
	"github.com/rafi/gits/internal/cli/exec"
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

// The -o flag of each line Bulk Command: table, the stock lines, or json, the
// envelope status and list share. One variable per command, since cobra
// binds a flag to exactly one destination.
var (
	cloneOutput  = "table"
	doctorOutput = "table"
	execOutput   = "table"
	fetchOutput  = "table"
	pullOutput   = "table"
	pushOutput   = "table"
)

// pushOpts is the vetted passthrough set from docs/adr/0002-push-safety-model.md.
// Nothing here reaches --force, -u or --mirror, and nothing should be added
// that does.
var pushOpts git.PushOptions

func init() {
	statusCmd.
		PersistentFlags().
		BoolVar(&statusOpts.Stat, "stat", false, "show HEAD± column with uncommitted line diffs")
	statusCmd.
		PersistentFlags().
		BoolVar(&statusOpts.Dirty, "dirty", false, "show only repos with uncommitted changes")
	statusCmd.
		PersistentFlags().
		BoolVar(&statusOpts.Unsynced, "unsynced", false, "show only repos ahead or behind upstream")

	// Every -o is registered here, with the styles its own command accepts.
	// `list` renders shapes a Bulk Command has no meaning for; the rest take
	// the pair bulk.ValidateFormat accepts. The values are the validators'
	// own, so help text, completion and what the command accepts cannot
	// disagree.
	for _, f := range []struct {
		cmd    *cobra.Command
		dst    *string
		styles []string
	}{
		{listCmd, &listOutput, list.Formats()},
		{cloneCmd, &cloneOutput, bulk.Formats()},
		{doctorCmd, &doctorOutput, bulk.Formats()},
		{execCmd, &execOutput, bulk.Formats()},
		{fetchCmd, &fetchOutput, bulk.Formats()},
		{pullCmd, &pullOutput, bulk.Formats()},
		{pushCmd, &pushOutput, bulk.Formats()},
		{statusCmd, &statusOutput, bulk.Formats()},
	} {
		registerOutputFlag(f.cmd, f.dst, f.styles)
	}

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
	rootCmd.AddCommand(doctorCmd)
	rootCmd.AddCommand(execCmd)
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

// registerOutputFlag wires one command's -o: the flag itself, its help text,
// and the shell completion that offers exactly the styles it accepts.
func registerOutputFlag(cmd *cobra.Command, dst *string, styles []string) {
	cmd.PersistentFlags().
		StringVarP(dst, "output", "o", *dst,
			fmt.Sprintf("output style (%s)", strings.Join(styles, ", ")))
	mustRegisterFlagCompletion(cmd, "output", completeValues(styles))
}

// mustRegisterFlagCompletion panics on a failure to register, which cobra
// reports only for a misspelled flag name or a second registration of the
// same flag. Both are wiring mistakes in this file, not runtime conditions,
// and either would otherwise show up as silently absent completion.
func mustRegisterFlagCompletion(cmd *cobra.Command, flag string, f cobra.CompletionFunc) {
	if err := cmd.RegisterFlagCompletionFunc(flag, f); err != nil {
		panic(err)
	}
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
	RunE: runWithDeps(func(args []string, deps types.RuntimeCLI) error {
		return clone.ExecClone(cloneOutput, args, deps)
	}),
}

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Report configuration and environment problems",
	Long: `Check the configuration and environment for problems.

Reports unknown config keys, project paths that do not exist, repositories
whose ` + "`dir:`" + ` cannot be resolved, the git and finder binaries, and the state
of each cached project. Exits non-zero if any finding is an error.

No Provider Source is ever contacted: every check is answered from the config
file, the filesystem and PATH, so this never waits on the network or prompts
for a token passphrase.`,
	Args: cobra.NoArgs,
	// The load-time warnings are among doctor's own findings, so the wrapper
	// must not also print them on Diagnostic Output.
	RunE: runWithDeps(func(args []string, deps types.RuntimeCLI) error {
		return doctor.ExecDoctor(doctorOutput, args, deps)
	}, reportsConfigWarnings()),
}

// execCmd takes everything after `--` as the command to run, and only what
// comes before it as the project and repository arguments — so a flag meant
// for the child (`gits exec acme -- git log -n 1`) is never parsed as one of
// gits' own. Cobra records the split point as ArgsLenAtDash.
var execCmd = &cobra.Command{
	Use:   "exec [project] [repo] -- <command> [args...]",
	Short: "Run a command in every repository",
	Long: `Run an arbitrary command in every repository of a project.

The command is run as-is, without a shell: write "sh -c '...'" yourself when
you need pipes or redirection. Each child runs with its repository as the
working directory and with GITS_PROJECT, GITS_REPO and GITS_REPO_PATH set.`,
	Args:                  cobra.ArbitraryArgs,
	DisableFlagsInUseLine: true,
	ValidArgsFunction:     completeProjectRepo,
	RunE: func(cmd *cobra.Command, args []string) error {
		dash := cmd.Flags().ArgsLenAtDash()
		if dash < 0 {
			// No `--` at all: there is no command to run, and guessing which
			// argument was meant to be one would be worse than saying so.
			return exec.ErrNoCommand
		}
		gitsArgs, command := args[:dash], args[dash:]
		return runWithDeps(func(_ []string, deps types.RuntimeCLI) error {
			return exec.Exec(execOutput, command, gitsArgs, deps)
		})(cmd, gitsArgs)
	},
}

var fetchCmd = &cobra.Command{
	Use:               "fetch [project] [repo]",
	Short:             "Fetch and prune from all remotes",
	Args:              cobra.MaximumNArgs(2),
	ValidArgsFunction: completeProjectRepo,
	RunE: runWithDeps(func(args []string, deps types.RuntimeCLI) error {
		return fetch.ExecFetch(fetchOutput, args, deps)
	}),
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
	RunE: runWithDeps(func(args []string, deps types.RuntimeCLI) error {
		return pull.ExecPull(pullOutput, args, deps)
	}),
}

var pushCmd = &cobra.Command{
	Use:               "push [project] [repo]",
	Short:             "Push current branch to its upstream",
	Args:              cobra.MaximumNArgs(2),
	ValidArgsFunction: completeProjectRepo,
	RunE: runWithDeps(func(args []string, deps types.RuntimeCLI) error {
		return push.ExecPush(pushOutput, pushOpts, args, deps)
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
