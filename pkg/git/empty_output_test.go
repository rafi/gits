package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestRefsEmptyRepoReturnsNoRefs proves Refs returns a zero-length slice (not a
// 1-element slice holding "") when a repo has no refs. Raw strings.Split on
// empty output yields [""], an off-by-one for len()-based consumers.
func TestRefsEmptyRepoReturnsNoRefs(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git executable not available: %v", err)
	}
	g, _ := NewGit()
	ctx := context.Background()
	dir := t.TempDir()
	if out, err := exec.CommandContext(ctx, "git", "-C", dir, "init", "-b", "main").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	got, err := g.Refs(ctx, dir)
	if err != nil {
		t.Fatalf("Refs: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("Refs on empty repo = %v (len %d), want len 0", got, len(got))
	}
}

// TestCommitDatesOutsideWindowReturnsNone proves CommitDates returns a
// zero-length slice when no commit falls inside the --since window.
func TestCommitDatesOutsideWindowReturnsNone(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git executable not available: %v", err)
	}
	g, _ := NewGit()
	ctx := context.Background()
	dir := t.TempDir()
	base := []string{
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
	}
	run := func(extraEnv []string, args ...string) {
		t.Helper()
		cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(), append(base, extraEnv...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run(nil, "init", "-b", "main")
	if err := os.WriteFile(filepath.Join(dir, "f"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(nil, "add", "f")
	// Date the commit well outside any sane --since window.
	old := []string{"GIT_AUTHOR_DATE=2000-01-01T00:00:00", "GIT_COMMITTER_DATE=2000-01-01T00:00:00"}
	run(old, "commit", "-m", "old")

	got, err := g.CommitDates(ctx, dir, "main", 1)
	if err != nil {
		t.Fatalf("CommitDates: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("CommitDates outside window = %v (len %d), want len 0", got, len(got))
	}
}
