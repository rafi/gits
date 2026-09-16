package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/rafi/gits/domain"

	"github.com/rafi/gits/internal/app/cli/commands/discover"
	"github.com/rafi/gits/internal/app/cli/commands/list"
	"github.com/rafi/gits/internal/app/cli/commands/status"
	"github.com/rafi/gits/internal/app/cli/render/output"
	"github.com/rafi/gits/internal/infra/git"
)

const (
	appName  = "gits"
	appShort = "gits is a tool for managing multiple Git repositories"
	appLong  = `Fast CLI Git manager for multiple repositories grouped by projects, with GitHub/GitLab/Bitbucket/Gitea/Forgejo/Gerrit support.`
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

// discoverOpts are the flags of `gits discover`.
var discoverOpts discover.Options

// tagValues holds each command's raw `--tag` values, keyed by command name.
var tagValues = map[string]*[]string{}

// pushOpts is the vetted passthrough set for safe bulk push.
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
	// the pair output.ValidateFormat accepts. The values are the validators'
	// own, so help text, completion and what the command accepts cannot
	// disagree.
	for _, f := range []struct {
		cmd    *cobra.Command
		dst    *string
		styles []string
	}{
		{listCmd, &listOutput, list.Formats()},
		{cloneCmd, &cloneOutput, output.Formats()},
		{doctorCmd, &doctorOutput, output.Formats()},
		{execCmd, &execOutput, output.Formats()},
		{fetchCmd, &fetchOutput, output.Formats()},
		{pullCmd, &pullOutput, output.Formats()},
		{pushCmd, &pushOutput, output.Formats()},
		{statusCmd, &statusOutput, output.Formats()},
	} {
		registerOutputFlag(f.cmd, f.dst, f.styles)
	}

	discoverFlags := discoverCmd.PersistentFlags()
	discoverFlags.IntVar(&discoverOpts.Min, "min", discover.DefaultMin,
		"minimum repositories a directory must hold to become a project")
	discoverFlags.BoolVarP(&discoverOpts.DryRun, "dry-run", "n", false,
		"report what would be added, write nothing")

	// `add` sets tags; the other commands filter by them.
	for _, cmd := range []*cobra.Command{
		checkoutCmd, cloneCmd, execCmd, fetchCmd, listCmd,
		orphanCmd, pullCmd, pushCmd, statusCmd,
	} {
		registerTagFlag(cmd, "only repositories carrying one of these tags (comma-separated)")
	}
	registerTagFlag(addCmd, "tag the recorded repositories (comma-separated)")

	pushFlags := pushCmd.PersistentFlags()
	pushFlags.BoolVar(&pushOpts.All, "all", false, "push all branches")
	pushFlags.BoolVar(&pushOpts.Branches, "branches", false, "push all branches (synonym of --all)")
	pushFlags.BoolVar(&pushOpts.AllTags, "all-tags", false,
		"push all git tags instead of the current branch")
	// Deprecated alias of --all-tags.
	pushFlags.BoolVar(&pushOpts.AllTags, "tags", false,
		"push all git tags instead of the current branch")
	mustDeprecateFlag(pushCmd, "tags", "use --all-tags instead")
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
	rootCmd.AddCommand(discoverCmd)
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

// registerTagFlag registers a command's repeatable, comma-separated `--tag/-t`
// flag and its completion.
func registerTagFlag(cmd *cobra.Command, usage string) {
	dst := new([]string)
	tagValues[cmd.Name()] = dst
	cmd.PersistentFlags().StringSliceVarP(dst, "tag", "t", nil, usage)
	mustRegisterFlagCompletion(cmd, "tag", completeTags)
}

// tagSet returns the parsed `--tag` values of the named command.
func tagSet(cmd string) domain.TagSet {
	dst, ok := tagValues[cmd]
	if !ok {
		return domain.TagSet{}
	}
	return domain.NewTagSet(*dst)
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

// mustDeprecateFlag marks a persistent flag deprecated, panicking on failure.
func mustDeprecateFlag(cmd *cobra.Command, flag, msg string) {
	if err := cmd.PersistentFlags().MarkDeprecated(flag, msg); err != nil {
		panic(err)
	}
}
