package main

import (
	"github.com/mitchellh/go-homedir"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/rafi/gits/internal/cache"
	"github.com/rafi/gits/internal/cli/config"
	"github.com/rafi/gits/internal/types"
	"github.com/rafi/gits/pkg/git"
)

var (
	configPath string
	configFile config.File
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
		BoolVarP(&configFile.Settings.Verbose, "verbose", "v", false, "display verbose output")

	cobra.OnInitialize(func() {
		if err := config.NewConfigFromFile(configPath, &configFile); err != nil {
			log.Warn(err)
		}
		setupLogger(configFile)
	})

	if err := rootCmd.Execute(); err != nil {
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
	return func(_ *cobra.Command, args []string) error {
		// Setup runtime dependencies.
		gitClient, err := git.NewGit()
		if err != nil {
			return err
		}
		cacheClient, err := cache.NewCacheClient("file")
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
		return f(args, types.RuntimeCLI{
			Theme:   theme,
			HomeDir: homeDir,
			Runtime: types.Runtime{
				Projects:   configFile.Projects,
				Settings:   configFile.Settings,
				ConfigPath: configFile.Filename,
				Git:        gitClient,
				Cache:      cacheClient,
			},
		})
	}
}
