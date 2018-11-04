package common

// RepoInfo represents a single repository
type RepoInfo map[string]string

// ProjectInfo represents a single project
type ProjectInfo struct {
	Path  string     `string:"path"`
	Repos []RepoInfo `mapstructure:"repos"`
}

// Config is the root of configuration
type Config struct {
	Projects map[string]ProjectInfo `mapstructure:"projects"`
	Verbose  bool
}
