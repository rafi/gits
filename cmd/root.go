package cmd

import (
	"fmt"
	"os"
	"path"
	"path/filepath"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cli"
	"github.com/rafi/gits/pkg/git"

	homedir "github.com/mitchellh/go-homedir"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	cfgFile string
	cfg     cli.Config
)

var rootCmd = &cobra.Command{
	Use:                    "gits",
	Short:                  "Gits is a manager for multiple Git repositories",
	Long:                   "A Fast CLI Git manager for multiple repositories",
	BashCompletionFunction: bashCompletionFunc,
}

// Execute is the entry-point of cobra
func Execute() {
	log.SetLevel(log.InfoLevel)

	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}

func loadGit() git.Git {
	gitPath := "git"
	git := git.New(gitPath)
	return git
}

func loadProjectsFromArgs(git git.Git, args []string) []domain.Project {
	projects := []domain.Project{}
	for _, projectName := range args {
		proj, err := getProject(projectName, cfg, git)
		if err != nil {
			log.Fatal(err)
		}
		projects = append(projects, proj)
	}

	return projects
}

// Get correctly returns a proper project object
func getProject(name string, config cli.Config, git git.Git) (domain.Project, error) {
	var (
		err       error
		project   domain.Project
		repoPaths []string
	)

	if name == "." || name[0:1] == "/" || name[0:2] == "./" {
		project.Path, err = filepath.Abs(name)
		if err != nil {
			return domain.Project{}, fmt.Errorf("getProject: %w", err)
		}
		project.Name = filepath.Base(project.Path)
		repoPaths, err = git.DiscoverRepos(project.Path)
		if err != nil {
			return domain.Project{}, fmt.Errorf("getProject: %w", err)
		}
		for _, repoPath := range repoPaths {
			repo := domain.Repository{"dir": repoPath}
			project.Repos = append(project.Repos, repo)
		}
	} else {
		project = config.Projects[name]
		project.Name = name
		project.AbsPath, err = homedir.Expand(project.Path)
		if err != nil {
			return domain.Project{}, fmt.Errorf("getProject: %w", err)
		}
	}

	return project, nil
}

func init() {
	cobra.OnInitialize(initApp)

	rootCmd.PersistentFlags().
		StringVar(&cfgFile, "config", "", "config file (default is $HOME/.gits.yaml)")

	rootCmd.PersistentFlags().
		BoolVarP(&cfg.Verbose, "verbose", "v", false, "display verbose output")
	_ = viper.BindPFlag("Verbose", rootCmd.PersistentFlags().Lookup("verbose"))
}

// initApp initializes the application.
func initApp() {
	if err := initConfig(); err != nil {
		log.Fatal(err)
	}
}

// initConfig reads in config file and ENV variables if set.
func initConfig() error {
	viper.SetDefault("Verbose", false)

	if len(cfgFile) > 0 {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory.
		home, err := homedir.Dir()
		if err != nil {
			return fmt.Errorf("Unable to find home directory: %w", err)
		}

		// Search config in home directory with filename
		viper.AddConfigPath(home)
		viper.SetConfigName(".gits")
		if xdgConfigHome := os.Getenv("XDG_CONFIG_HOME"); xdgConfigHome != "" {
			viper.AddConfigPath(path.Join(xdgConfigHome, "gits"))
		} else {
			viper.AddConfigPath(path.Join(home, ".config", "gits"))
		}
	}

	viper.SetConfigType("yaml")
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		return fmt.Errorf("Unable to load config file: %w", err)
	}

	if err := viper.Unmarshal(&cfg); err != nil {
		return fmt.Errorf("Unable to parse config file: %w", err)
	}

	if cfg.Verbose {
		log.SetLevel(log.DebugLevel)
		log.WithField("config", viper.ConfigFileUsed()).Debug("Loaded config")
	}
	return nil
}
