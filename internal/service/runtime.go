// Package service holds what the business layer runs with: the dependencies
// every command is handed, independent of how the user reached it.
package service

import (
	"context"
	"log/slog"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cache"
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
