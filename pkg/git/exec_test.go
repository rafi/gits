package git

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// TestSnapshotIgnoresStderr proves stderr noise emitted during a successful
// `git status` (forced here via GIT_TRACE) never leaks into the porcelain
// parse: a clean worktree must report zero staged, unstaged and untracked
// entries.
func TestSnapshotIgnoresStderr(t *testing.T) {
	dir := setupRepo(t)
	t.Setenv("GIT_TRACE", "1")

	g := NewGit()
	snap, err := g.Snapshot(context.Background(), dir)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if snap.Staged != 0 || snap.Unstaged != 0 || snap.Untracked != 0 {
		t.Errorf("Snapshot on clean repo with stderr noise = %+v, want zeros", snap.WorkTree)
	}
}

// TestExecErrorKeepsStderr proves failures still fold git's stderr text into
// the returned error, so users see the actual git message.
func TestExecErrorKeepsStderr(t *testing.T) {
	dir := setupRepo(t)

	g := NewGit()
	_, err := g.Exec(context.Background(), dir, []string{"log", "no-such-ref"})
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
	// No skip guard: the precondition is checked before git is ever invoked,
	// which is the point of the sentinel.
	g := NewGit()
	target := t.TempDir() // exists already
	if _, err := g.Clone(context.Background(), "unused", target); !errors.Is(err, ErrTargetExists) {
		t.Errorf("Clone onto existing dir = %v, want ErrTargetExists", err)
	}
}
