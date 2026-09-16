package cli

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/rafi/gits/internal/app"
	"github.com/rafi/gits/internal/app/cli/commands/add"
	"github.com/rafi/gits/internal/app/cli/commands/browse"
	"github.com/rafi/gits/internal/app/cli/commands/cd"
	"github.com/rafi/gits/internal/app/cli/commands/checkout"
	"github.com/rafi/gits/internal/app/cli/commands/clone"
	"github.com/rafi/gits/internal/app/cli/commands/discover"
	"github.com/rafi/gits/internal/app/cli/commands/doctor"
	"github.com/rafi/gits/internal/app/cli/commands/exec"
	"github.com/rafi/gits/internal/app/cli/commands/fetch"
	"github.com/rafi/gits/internal/app/cli/commands/list"
	"github.com/rafi/gits/internal/app/cli/commands/orphan"
	"github.com/rafi/gits/internal/app/cli/commands/pull"
	"github.com/rafi/gits/internal/app/cli/commands/push"
	"github.com/rafi/gits/internal/app/cli/commands/status"
	"github.com/rafi/gits/internal/app/cli/commands/sync"
	"github.com/rafi/gits/internal/version"
)

var addCmd = &cobra.Command{
	Use:   "add [project] [repository]...",
	Short: "Add cloned repositories to a project's list",
	Long: `Record already-cloned repositories under a project's ` + "`repos:`" + ` in the
config file. The project is created when it does not exist, and so is the
config file itself when you have none.

Each repository argument is a directory, a glob pattern, or a clone URL:

  gits add acme                              # the current directory
  gits add acme ./api ../shared/tools        # directories
  gits add acme 'backend*'                   # every repository matching a glob
  gits add acme git@github.com:acme/api.git  # cloned here first, then added

This is for a project that lists its repositories by hand. A project that
discovers them from a source (github, gitlab, bitbucket, or a filesystem path)
has nothing to add to: use ` + "`gits orphan`" + ` to see what its directory holds
that no project declares.`,
	Args:              cobra.ArbitraryArgs,
	ValidArgsFunction: completeAddArgs,
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
	RunE: runWithDeps(func(args []string, deps app.RuntimeCLI) error {
		return clone.ExecClone(cloneOutput, args, deps)
	}),
}

var discoverCmd = &cobra.Command{
	Use:   "discover <path>",
	Short: "Find groups of repositories and add them as projects",
	Long: `Search a path for directories that hold git repositories, and record each
one as a project in the config file.

The search is recursive and goes as deep as your tree does. Every directory
holding repositories becomes a project of exactly the ones inside it, named
after that directory:

  gits discover ~/src         # every directory under ~/src
  gits discover .             # the current directory
  gits discover ~/src -n      # report what it would add, write nothing
  gits discover ~/src --min 2 # skip directories holding a single repository

A ` + "`~/src`" + ` holding ` + "`project1/*`" + `, ` + "`project2/*`" + ` and a couple of loose clones gives
three projects: ` + "`project1`" + `, ` + "`project2`" + `, and ` + "`src`" + ` with the two strays. Each one
names its repositories rather than searching its path, so ` + "`src`" + ` holds its own
two and does not reach down into the others.

Each repository is recorded with the remote it was cloned from, the same way
` + "`gits add`" + ` records one.

Running this again after cloning more is safe, and finds what is new. A
project that discovers its repositories — from a forge, or by searching a
directory — owns its whole path, so nothing under it is ever written. A
project that lists them by hand is looked inside all the same: clones that
appeared under its path since it was written join that project, under the
name the config already gives it, rather than becoming a project of their own.

Two directories of the same name become two projects: the second is named
after its parent, ` + "`work-archive`" + ` beside ` + "`archive`" + `, since a config file cannot
hold one name twice. A name another project already uses is reported rather
than quietly renamed. A repository nested inside another is left alone — use
` + "`gits orphan`" + ` to find those.`,
	// A missing path gets its own error; too many args keep cobra's.
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return discover.ErrNoPath
		}
		return cobra.ExactArgs(1)(cmd, args)
	},
	ValidArgsFunction: completeDirs,
	RunE: runWithDeps(func(args []string, deps app.RuntimeCLI) error {
		return discover.ExecDiscover(discoverOpts, args, deps)
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
	RunE: runWithDeps(func(args []string, deps app.RuntimeCLI) error {
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
		return runWithDeps(func(_ []string, deps app.RuntimeCLI) error {
			return exec.Exec(execOutput, command, gitsArgs, deps)
		})(cmd, gitsArgs)
	},
}

var fetchCmd = &cobra.Command{
	Use:               "fetch [project] [repo]",
	Short:             "Fetch and prune from all remotes",
	Args:              cobra.MaximumNArgs(2),
	ValidArgsFunction: completeProjectRepo,
	RunE: runWithDeps(func(args []string, deps app.RuntimeCLI) error {
		return fetch.ExecFetch(fetchOutput, args, deps)
	}),
}

var listCmd = &cobra.Command{
	Use:               "list [project]...",
	Short:             "List project repositories",
	Aliases:           []string{"ls"},
	Args:              cobra.ArbitraryArgs,
	ValidArgsFunction: completeProject,
	RunE: runWithDeps(func(args []string, deps app.RuntimeCLI) error {
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
	RunE: runWithDeps(func(args []string, deps app.RuntimeCLI) error {
		return pull.ExecPull(pullOutput, args, deps)
	}),
}

var pushCmd = &cobra.Command{
	Use:               "push [project] [repo]",
	Short:             "Push current branch to its upstream",
	Args:              cobra.MaximumNArgs(2),
	ValidArgsFunction: completeProjectRepo,
	RunE: runWithDeps(func(args []string, deps app.RuntimeCLI) error {
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
	Use:               "status [project|path] [repo]",
	Short:             "Show Git repositories short status",
	Args:              cobra.MaximumNArgs(2),
	ValidArgsFunction: completeProjectRepo,
	RunE: runWithDeps(func(args []string, deps app.RuntimeCLI) error {
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
