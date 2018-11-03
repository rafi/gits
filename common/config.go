package common

import (
	"github.com/spf13/viper"
)

type RepoInfo map[string]string

type ProjectInfo struct {
	Path  string     `"path"`
	Repos []RepoInfo `mapstructure:"repos"`
}

type GmuxConfig struct {
	Projects map[string]ProjectInfo `mapstructure:"projects"`
}

func GetProject(name string) ProjectInfo {
	var cfg GmuxConfig
	err := viper.Unmarshal(&cfg)
	if err != nil {
		panic(err)
	}

	return cfg.Projects[name]
}
