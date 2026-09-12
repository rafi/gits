package git

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// TestNewGitDoesNotProbePath proves constructing the client never looks git
// up: with PATH emptied, NewGit still hands back a usable client, so commands
// that never shell out are unaffected by git's absence.
//
// t.Setenv is incompatible with t.Parallel; the tests here must stay serial.
func TestNewGitDoesNotProbePath(t *testing.T) {
	t.Setenv("PATH", "")

	g := NewGit()
	// IsRepo is a stat, not a subprocess, and must keep working.
	if g.IsRepo(context.Background(), t.TempDir()) {
		t.Error("IsRepo on a non-repo dir = true, want false")
	}
}

// TestExecWithoutGitBinary proves an operation that does shell out fails on
// its own, with a matchable sentinel rather than a bare exec error.
//
//nolint:paralleltest // empties PATH with t.Setenv, which is incompatible with t.Parallel.
func TestExecWithoutGitBinary(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", "")
	g := NewGit()
	ctx := context.Background()

	t.Run("Exec path", func(t *testing.T) {
		_, err := g.Remote(ctx, dir)
		if !errors.Is(err, ErrGitNotFound) {
			t.Fatalf("Remote without git on PATH = %v, want ErrGitNotFound", err)
		}
		// The operation's own wrapping must survive the translation.
		if !strings.Contains(err.Error(), "unable to get remote URL") {
			t.Errorf("error %q lost Remote's own context", err)
		}
	})

	t.Run("ExecCombined path", func(t *testing.T) {
		if _, err := g.Fetch(ctx, dir); !errors.Is(err, ErrGitNotFound) {
			t.Fatalf("Fetch without git on PATH = %v, want ErrGitNotFound", err)
		}
	})
}

// TestErrGitNotFoundNamesTheProblem proves the message a user ends up reading
// names git and PATH, instead of reading like a Go runtime detail.
func TestErrGitNotFoundNamesTheProblem(t *testing.T) {
	t.Parallel()

	msg := ErrGitNotFound.Error()
	if !strings.Contains(msg, "git") {
		t.Errorf("ErrGitNotFound = %q, want it to name git", msg)
	}
	if !strings.Contains(msg, "PATH") {
		t.Errorf("ErrGitNotFound = %q, want it to name PATH", msg)
	}
}
