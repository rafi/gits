package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"runtime"
	"sync"

	"charm.land/lipgloss/v2"
	"github.com/mitchellh/go-homedir"
	"github.com/spf13/cobra"

	"github.com/rafi/gits/internal/cache"
	"github.com/rafi/gits/internal/cli/config"
	"github.com/rafi/gits/internal/git"
	"github.com/rafi/gits/internal/logging"
	"github.com/rafi/gits/internal/types"
	"github.com/rafi/gits/internal/version"
)

var (
	configPath  string
	configFile  config.File
	verboseFlag bool
	// errConfigLoad holds a config parse or read failure from OnInitialize.
	// A cobra initializer cannot return an error, so it is surfaced by the
	// root's PersistentPreRunE before any command runs. A missing config file
	// is not an error and leaves this nil.
	errConfigLoad error
	// logger is the debug tracer built once from the resolved config and
	// handed to every command on types.Runtime. It is nil until cobra's
	// initializer runs; newRuntime substitutes a discarding logger so a
	// command driven directly by a test never panics on it.
	logger *slog.Logger
)

// rootCmd represents gits base command.
var rootCmd = &cobra.Command{
	Use:           appName,
	Short:         appShort,
	Long:          appLong,
	Version:       fmt.Sprintf("%s %s", version.GetVersion(), runtime.Version()),
	SilenceUsage:  true,
	SilenceErrors: true,
	// A config file that exists but fails to parse is fatal: without it the
	// Projects map is empty and every command would misreport the reason.
	// This runs for every subcommand, since none overrides it.
	PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
		return errConfigLoad
	},
}

// main runs the root gits command.
func main() {
	registerRootFlags()

	cobra.OnInitialize(func() {
		// A read or parse failure of a config file that exists is captured and
		// turned into a non-zero exit by PersistentPreRunE. A missing file
		// returns nil, so git-free commands (add, version) still run.
		errConfigLoad = config.NewConfigFromFile(configPath, &configFile)
		// The CLI flag wins over the config file's settings.verbose, which
		// loadConfig would otherwise clobber by unmarshalling onto Settings.
		if verboseFlag {
			configFile.Settings.Verbose = true
		}
		logger = newLogger(configFile)
	})

	if err := execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// registerRootFlags wires the persistent flags and version output onto
// rootCmd. Extracted from main so tests can drive the assembled command;
// guarded so a second call (a test after main-style setup) is a no-op rather
// than a pflag duplicate-registration panic.
func registerRootFlags() {
	rootFlagsOnce.Do(func() {
		// --version prints the same string as `gits version`, which stays as
		// an alias. The default -v shorthand is taken by --verbose, so
		// --version has none.
		rootCmd.SetVersionTemplate("gits {{.Version}}\n")

		rootCmd.PersistentFlags().
			StringVarP(&configPath, "config", "c", "", "config file (default is $HOME/.gits.yaml)")

		// -C rejects anything but auto|always|never, so a typo fails loudly
		// rather than silently meaning auto.
		rootCmd.PersistentFlags().
			VarP(config.NewColorValue(&configFile.Color), "color", "C", "color (auto, always, never)")

		rootCmd.PersistentFlags().
			BoolVarP(&verboseFlag, "verbose", "v", false, "display verbose output")
	})
}

var rootFlagsOnce sync.Once

// execute runs the command tree and returns rather than exiting, so the
// signal handler is released before main reports the failure: [log.Fatal]
// exits the process and would skip the deferred stop().
func execute() error {
	// Root context canceled on SIGINT so in-flight git operations stop
	// promptly on Ctrl-C; per-op timeouts derive from this context.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	return rootCmd.ExecuteContext(ctx)
}

// newLogger builds the process-wide debug tracer from the resolved config.
// It writes to Diagnostic Output because that is where anything that is not
// the command's result belongs; a default run emits nothing below warn level.
func newLogger(cfg config.File) *slog.Logger {
	l := logging.New(os.Stderr, cfg.Settings.Verbose)
	l.Debug("loaded config file", "path", cfg.Filename)
	return l
}

// newRuntime builds the shared runtime dependencies for a command. It cannot
// fail: nothing here reaches outside the process, and in particular the git
// client no longer probes for its executable — so a missing git stops the
// operations that need it, not every command.
//
// It returns any warnings gathered while resolving settings (an unparseable
// duration falls back to its default and is reported here rather than logged
// from the domain package). The caller decides where they surface: a command
// writes them to Diagnostic Output; shell completion discards them so Tab
// stays silent.
func newRuntime(ctx context.Context) (types.Runtime, []error) {
	// A command reached without main's initializer — a test driving the
	// assembled cobra tree — still gets a working, silent logger.
	log := logging.Or(logger)

	gitClient := git.NewGit()
	gitClient.SetLogger(log)

	var warnings []error
	gitTimeout, err := configFile.Settings.GitTimeoutDuration()
	if err != nil {
		warnings = append(warnings, err)
	}
	gitClient.SetNetworkTimeout(gitTimeout)

	cacheTTL, err := configFile.Settings.CacheTTLDuration()
	if err != nil {
		warnings = append(warnings, err)
	}

	// providerTimeout is consulted deeper in the loader on a provider fetch;
	// validate it once here so a bad value is reported alongside the others
	// rather than swallowed where the loader reads it.
	if _, err := configFile.Settings.ProviderTimeoutDuration(); err != nil {
		warnings = append(warnings, err)
	}

	return types.Runtime{
		Ctx:        ctx,
		Projects:   configFile.Projects,
		Settings:   configFile.Settings,
		ConfigPath: configFile.Filename,
		Git:        &gitClient,
		Cache:      cache.NewFileCache(cacheTTL, log),
		Log:        log,
	}, warnings
}

// runWithDeps execute a command with dependencies.
func runWithDeps(f func([]string, types.RuntimeCLI) error) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		// Setup runtime dependencies.
		runtime, settingWarnings := newRuntime(cmd.Context())
		homeDir, err := homedir.Dir()
		if err != nil {
			return err
		}

		// Setup CLI theme.
		theme := config.NewThemeDefault()
		if err := theme.ParseConfig(configFile.Settings.Theme); err != nil {
			return err
		}

		// A setting that fell back to its default, or a deprecated config key,
		// is a warning, not a failure: render it as prose on Diagnostic Output,
		// styled like any downgraded warning, rather than as a structured log
		// record or a raw os.Stderr print.
		for _, w := range configFile.Warnings {
			writeWarning(os.Stderr, theme, w)
		}
		for _, w := range settingWarnings {
			writeWarning(os.Stderr, theme, w.Error())
		}

		// Run command with dependencies.
		cmdErr := f(args, types.RuntimeCLI{
			Theme:   theme,
			HomeDir: homeDir,
			Out:     os.Stdout,
			Err:     os.Stderr,
			Runtime: runtime,
		})

		// Downgrade warnings to a subtle sentence on Diagnostic Output — real
		// errors must still propagate.
		if types.IsWarning(cmdErr) {
			writeWarning(os.Stderr, theme, cmdErr.Error())
			return nil
		}
		return cmdErr
	}
}

// writeWarning renders a warning as a plain sentence on Diagnostic Output,
// styled with the theme's Warning style. It is the one place downgraded
// warnings — a passed-over Repository, a setting that fell back to its
// default — reach the user, so none of them arrive as a `level=warning`
// log record.
func writeWarning(w io.Writer, theme config.Theme, msg string) {
	lipgloss.Fprintln(w, theme.Warning.Render(msg))
}
