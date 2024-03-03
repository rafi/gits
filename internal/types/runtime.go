package types

import (
	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cache"
	"github.com/rafi/gits/internal/cli/config"
	"github.com/rafi/gits/pkg/git"
)

// Runtime is the runtime dependencies for the application.
type Runtime struct {
	Projects   domain.ProjectListKeyed
	Cache      cache.Cacher
	ConfigPath string
	Git        git.Git
	Settings   domain.Settings
}

// RuntimeCLI is the runtime dependencies for the CLI client.
type RuntimeCLI struct {
	Theme   config.Theme
	HomeDir string

	Runtime
}
