package main

import (
	"github.com/mitchellh/go-homedir"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/rafi/gits/internal/cli"
	"github.com/rafi/gits/internal/config"
	"github.com/rafi/gits/pkg/git"
)

var (
	configPath string
	configFile config.File
)

// rootCmd represents gits base command.
var rootCmd = &cobra.Command{
	Use:   appName,
	Short: appShort,
	Long:  appLong,
}

// main runs the root gits command.
func main() {
	rootCmd.PersistentFlags().
		StringVar(&configPath, "config", "", "config file (default is $HOME/.gits.yaml)")

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
	logLevel := log.InfoLevel
	if cfg.Settings.Verbose {
		logLevel = log.DebugLevel
	}
	log.SetLevel(logLevel)
	log.Debugf("Log level set to %s", log.GetLevel())
	log.WithField("config", cfg.Filename).Debug("Loading config file")
}

// runWithDeps execute a command with dependencies.
func runWithDeps(f func([]string, cli.RuntimeDeps) error) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		homeDir, err := homedir.Dir()
		if err != nil {
			return err
		}
		gitClient, err := git.NewGit()
		if err != nil {
			log.Fatal(err)
		}
		theme := cli.NewThemeDefault()
		if err := theme.ParseConfig(configFile.Settings.Theme); err != nil {
			log.Fatal(err)
		}

		return f(args, cli.RuntimeDeps{
			Settings: configFile.Settings,
			Git:      gitClient,
			Theme:    theme,
			HomeDir:  homeDir,
			Projects: configFile.Projects,
			Source:   configFile.Filename,
		})
	}
}
