package main

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/rafi/gits/internal/cli/config"
	"github.com/rafi/gits/internal/git"
)

// TestRuntimeUsableWithoutGit proves the git binary's absence is no longer a
// startup gate. Every command builds its runtime through newRuntime, so a
// failure here killed even the commands that never shell out to git. The
// client it hands back must be usable: git-free work succeeds, and work that
// does shell out fails on its own with a matchable reason.
//
// t.Setenv is incompatible with t.Parallel; this test must stay serial.
func TestRuntimeUsableWithoutGit(t *testing.T) {
	t.Setenv("PATH", "")
	ctx := context.Background()

	rt, _ := newRuntime(ctx)
	if rt.Git == nil {
		t.Fatal("newRuntime without git on PATH gave a nil git client")
	}

	// IsRepo is a stat, not a subprocess — unaffected by git's absence.
	if rt.Git.IsRepo(ctx, t.TempDir()) {
		t.Error("IsRepo on a non-repo dir = true, want false")
	}

	// Remote does shell out, and must fail only itself.
	if _, err := rt.Git.Remote(ctx, t.TempDir()); !errors.Is(err, git.ErrGitNotFound) {
		t.Errorf("Remote without git on PATH = %v, want ErrGitNotFound", err)
	}
}

// TestRuntimeLoggerAlwaysPresent proves the nil guard at the one place every
// command passes through: newRuntime substitutes a discarding logger when
// cobra's initializer has not run — a test driving a command directly — so
// deps.Log is never nil in a command's hands.
//
// It swaps the package-level logger, so it must stay serial.
//
//nolint:paralleltest // mutates the package-level logger.
func TestRuntimeLoggerAlwaysPresent(t *testing.T) {
	logger = nil
	t.Cleanup(func() { logger = nil })

	rt, _ := newRuntime(context.Background())
	if rt.Log == nil {
		t.Fatal("newRuntime gave a nil logger, want a discarding one")
	}
	rt.Log.Debug("must not panic")
}

// TestNewLoggerVerbosity proves -v / settings.verbose is what decides whether
// debug tracing appears, at the point the flag and the config file have
// already been reconciled.
func TestNewLoggerVerbosity(t *testing.T) {
	t.Parallel()

	for _, verbose := range []bool{false, true} {
		var cfg config.File
		cfg.Settings.Verbose = verbose
		got := newLogger(cfg).Enabled(t.Context(), slog.LevelDebug)
		if got != verbose {
			t.Errorf("newLogger(verbose=%v) debug enabled = %v, want %v",
				verbose, got, verbose)
		}
	}
}
