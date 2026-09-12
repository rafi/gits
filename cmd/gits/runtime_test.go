package main

import (
	"context"
	"errors"
	"testing"

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

	rt := newRuntime(ctx)
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
