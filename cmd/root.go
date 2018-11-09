package cmd

import (
	homedir "github.com/mitchellh/go-homedir"
	"github.com/rafi/gits/common"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"os"
	"path"
)

var cfgFile string

var cfg common.Config

var rootCmd = &cobra.Command{
	Use:                    "gits",
	Short:                  "Gits is a manager for multiple Git repositories",
	Long:                   "A Fast CLI Git manager for multiple repositories",
	BashCompletionFunction: bashCompletionFunc,
	// Uncomment the following line if your bare application
	// has an action associated with it:
	//	Run: func(cmd *cobra.Command, args []string) { },
}

// Execute is the entry-point of cobra
func Execute() {
	log.SetLevel(log.InfoLevel)

	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.gits.yaml)")

	rootCmd.PersistentFlags().BoolVarP(&cfg.Verbose, "verbose", "v", false, "display verbose output")
	_ = viper.BindPFlag("Verbose", rootCmd.PersistentFlags().Lookup("verbose"))
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	viper.SetDefault("Verbose", false)

	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory.
		home, err := homedir.Dir()
		if err != nil {
			log.Fatal("Unable to find home directory, ", err)
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
		log.Error("Unable to load config file, ", err)
	}
	_ = viper.Unmarshal(&cfg)
	if cfg.Verbose {
		log.SetLevel(log.DebugLevel)
		log.WithField("config", viper.ConfigFileUsed()).Debug("Loaded config")
	}
}

const (
	bashCompletionFunc = `__gits_get_projects()
{
    local gits_output out
    if gits_output=$(gits list 2>/dev/null); then
        out=($(echo "${gits_output}" | awk '{print $1}'))
        COMPREPLY=( $( compgen -W "${out[*]}" -- "$cur" ) )
    fi
    if [[ $? -eq 0 ]]; then
        return 0
    fi
}

__custom_func() {
    case ${last_command} in
        gits_checkout | gits_clone | gits_fetch | gits_status)
            __gits_get_projects
            return
            ;;
        *)
            ;;
    esac
}
`
)
