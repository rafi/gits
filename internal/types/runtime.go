package types

import (
	"context"
	"io"

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

	// Out is the Result Output destination: what the command was asked for —
	// the status table, the JSON document, the list of names — and nothing
	// else, so any command's output can be piped or redirected unfiltered.
	Out io.Writer
	// Err is the Diagnostic Output destination: everything a command emits
	// about producing its Result Output — live per-repository progress, the
	// summary footer, the error epilogue.
	//
	// The progress reporter is derived from this writer rather than injected:
	// walk.NewReporter sniffs it for a terminal, so a test buffer yields the
	// no-op reporter and no ANSI reaches the assertions.
	Err io.Writer

	Runtime
}
