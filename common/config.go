package common

import (
	"github.com/spf13/viper"
)

// RepoInfo represents a single repository
type RepoInfo map[string]string

// ProjectInfo represents a single project
type ProjectInfo struct {
	Path  string     `string:"path"`
	Repos []RepoInfo `mapstructure:"repos"`
}

// GmuxConfig is the root of configuration
type GmuxConfig struct {
	Projects map[string]ProjectInfo `mapstructure:"projects"`
}

// GetProject returns the config struct of provided project name
func GetProject(name string) ProjectInfo {
	var cfg GmuxConfig
	err := viper.Unmarshal(&cfg)
	if err != nil {
		panic(err)
	}

	return cfg.Projects[name]
}
