package types

import (
	"context"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cache"
	"github.com/rafi/gits/internal/cli/config"
	"github.com/rafi/gits/pkg/git"
)

// Runtime is the runtime dependencies for the application.
type Runtime struct {
	// Ctx is the SIGINT-aware root context. Local git operations and the
	// GitHub and GitLab remote fetches derive from it, so Ctrl-C cancels that
	// in-flight work. Bitbucket repository listing is the exception: its SDK
	// accepts no context and cannot be interrupted.
	Ctx        context.Context
	Projects   domain.ProjectListKeyed
	Cache      cache.Cacher
	ConfigPath string
	Git        git.GitClient
	Settings   domain.Settings
}

// RuntimeCLI is the runtime dependencies for the CLI client.
type RuntimeCLI struct {
	Theme   config.Theme
	HomeDir string

	Runtime
}
