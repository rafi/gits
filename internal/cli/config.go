package cli

import "github.com/rafi/gits/domain"

// Config is the root of configuration
type Config struct {
	Projects map[string]domain.Project `mapstructure:"projects"`
	Verbose  bool
}
