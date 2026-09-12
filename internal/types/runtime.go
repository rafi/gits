package types

import (
	"context"
	"io"
	"log/slog"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cache"
	"github.com/rafi/gits/internal/cli/config"
	"github.com/rafi/gits/internal/git"
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
	Git        git.Client
	Settings   domain.Settings

	// ConfigWarnings are the non-fatal notices gathered while loading the
	// config file — an unknown key, a deprecated one, a setting that fell
	// back to its default. Every command already shows them once on
	// Diagnostic Output; they are carried here so `gits doctor` can report
	// them as findings rather than re-reading and re-parsing the file to
	// rediscover what the loader already knows.
	ConfigWarnings []string

	// Log is the debug tracer: page fetches, cache hits and misses, git's
	// stderr. It is never how a user is told something — that is Diagnostic
	// Output on Err, as prose. Constructed once in cmd/gits and passed here
	// so no package reaches for a global logger.
	Log *slog.Logger
}

// RuntimeCLI is the runtime dependencies for the CLI client.
type RuntimeCLI struct {
	Runtime

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
	// the bulk module sniffs it for a terminal, so a test buffer yields the
	// no-op reporter and no ANSI reaches the assertions.
	Err io.Writer
}
