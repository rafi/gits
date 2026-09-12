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
	"github.com/rafi/gits/internal/types"
	"github.com/rafi/gits/pkg/git"
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

	// Root context cancelled on SIGINT so in-flight git operations stop
	// promptly on Ctrl-C; per-op timeouts derive from this context.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if err := rootCmd.ExecuteContext(ctx); err != nil {
		log.Fatal(err)
	}
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

// runWithDeps execute a command with dependencies.
func runWithDeps(f func([]string, types.RuntimeCLI) error) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		// Setup runtime dependencies.
		gitClient, err := git.NewGit()
		if err != nil {
			return err
		}
		gitClient.SetNetworkTimeout(configFile.Settings.GitTimeoutDuration())
		cacheClient, err := cache.NewCacheClient("file", configFile.Settings.CacheTTLDuration())
		if err != nil {
			return err
		}
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
			Runtime: types.Runtime{
				Ctx:        cmd.Context(),
				Projects:   configFile.Projects,
				Settings:   configFile.Settings,
				ConfigPath: configFile.Filename,
				Git:        &gitClient,
				Cache:      cacheClient,
			},
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
