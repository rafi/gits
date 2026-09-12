package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/mitchellh/go-homedir"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/rafi/gits/internal/cache"
	"github.com/rafi/gits/internal/cli/config"
	"github.com/rafi/gits/internal/git"
	"github.com/rafi/gits/internal/types"
)

var (
	configPath  string
	configFile  config.File
	verboseFlag bool
)

// rootCmd represents gits base command.
var rootCmd = &cobra.Command{
	Use:           appName,
	Short:         appShort,
	Long:          appLong,
	SilenceUsage:  true,
	SilenceErrors: true,
}

// main runs the root gits command.
func main() {
	rootCmd.PersistentFlags().
		StringVarP(&configPath, "config", "c", "", "config file (default is $HOME/.gits.yaml)")

	rootCmd.PersistentFlags().
		StringVarP(&configFile.Color, "color", "C", config.ColorOptionDefault, "color")

	rootCmd.PersistentFlags().
		BoolVarP(&verboseFlag, "verbose", "v", false, "display verbose output")

	cobra.OnInitialize(func() {
		if err := config.NewConfigFromFile(configPath, &configFile); err != nil {
			log.Warn(err)
		}
		// The CLI flag wins over the config file's settings.verbose, which
		// loadConfig would otherwise clobber by unmarshalling onto Settings.
		if verboseFlag {
			configFile.Settings.Verbose = true
		}
		setupLogger(configFile)
	})

	if err := execute(); err != nil {
		log.Fatal(err)
	}
}

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

// setupLogger configures gits logger and sets the verbosity level.
func setupLogger(cfg config.File) {
	log.SetFormatter(&log.TextFormatter{
		DisableTimestamp:       true,
		DisableLevelTruncation: true,
		QuoteEmptyFields:       true,
	})
	logLevel := log.InfoLevel
	if cfg.Settings.Verbose {
		logLevel = log.DebugLevel
	}
	log.SetLevel(logLevel)
	log.Debugf("Log level set to %s", log.GetLevel())
	log.WithField("config", cfg.Filename).Debug("Loading config file")
}

// newRuntime builds the shared runtime dependencies for a command. It cannot
// fail: nothing here reaches outside the process, and in particular the git
// client no longer probes for its executable — so a missing git stops the
// operations that need it, not every command.
func newRuntime(ctx context.Context) types.Runtime {
	gitClient := git.NewGit()
	gitClient.SetNetworkTimeout(configFile.Settings.GitTimeoutDuration())
	return types.Runtime{
		Ctx:        ctx,
		Projects:   configFile.Projects,
		Settings:   configFile.Settings,
		ConfigPath: configFile.Filename,
		Git:        &gitClient,
		Cache:      cache.NewFileCache(configFile.Settings.CacheTTLDuration()),
	}
}

// runWithDeps execute a command with dependencies.
func runWithDeps(f func([]string, types.RuntimeCLI) error) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		// Setup runtime dependencies.
		runtime := newRuntime(cmd.Context())
		homeDir, err := homedir.Dir()
		if err != nil {
			return err
		}

		// Setup CLI theme.
		theme := config.NewThemeDefault()
		if err := theme.ParseConfig(configFile.Settings.Theme); err != nil {
			return err
		}

		// Run command with dependencies.
		cmdErr := f(args, types.RuntimeCLI{
			Theme:   theme,
			HomeDir: homeDir,
			Out:     os.Stdout,
			Err:     os.Stderr,
			Runtime: runtime,
		})

		// Downgrade warnings to a subtle log line — real errors must still
		// propagate.
		if types.IsWarning(cmdErr) {
			log.Warn(cmdErr.Error())
			return nil
		}
		return cmdErr
	}
}
