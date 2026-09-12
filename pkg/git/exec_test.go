package git

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// TestWorkingStateIgnoresStderr proves stderr noise emitted during a
// successful `git status` (forced here via GIT_TRACE) never leaks into the
// porcelain parse: a clean worktree must report zero staged, unstaged and
// untracked entries.
func TestWorkingStateIgnoresStderr(t *testing.T) {
	dir := setupRepo(t)
	t.Setenv("GIT_TRACE", "1")

	g, err := NewGit()
	if err != nil {
		t.Fatalf("NewGit: %v", err)
	}
	wt, err := g.WorkingState(context.Background(), dir)
	if err != nil {
		t.Fatalf("WorkingState: %v", err)
	}
	if wt.Staged != 0 || wt.Unstaged != 0 || wt.Untracked != 0 {
		t.Errorf("WorkingState on clean repo with stderr noise = %+v, want zeros", wt)
	}
}

// TestExecErrorKeepsStderr proves failures still fold git's stderr text into
// the returned error, so users see the actual git message.
func TestExecErrorKeepsStderr(t *testing.T) {
	dir := setupRepo(t)

	g, err := NewGit()
	if err != nil {
		t.Fatalf("NewGit: %v", err)
	}
	_, err = g.Exec(context.Background(), dir, []string{"log", "no-such-ref"})
	if err == nil {
		t.Fatal("Exec(log no-such-ref) succeeded, want error")
	}
	if !strings.Contains(err.Error(), "no-such-ref") {
		t.Errorf("error %q does not include git's stderr message", err)
	}
}

// TestCloneTargetExists proves the "already cloned" precondition lives in
// Git.Clone as a matchable sentinel rather than duplicated stat checks in
// callers.
func TestCloneTargetExists(t *testing.T) {
	g, err := NewGit()
	if err != nil {
		t.Skipf("git executable not available: %v", err)
	}
	target := t.TempDir() // exists already
	if _, err := g.Clone(context.Background(), "unused", target); !errors.Is(err, ErrTargetExists) {
		t.Errorf("Clone onto existing dir = %v, want ErrTargetExists", err)
	}
}
