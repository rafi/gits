package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// snapshotRun executes git in dir, failing the test on error.
func snapshotRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), "git",
		append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// TestSnapshot proves one porcelain-v2 pass yields branch, upstream,
// ahead/behind and worktree counts that previously required four separate
// git invocations.
func TestSnapshot(t *testing.T) {
	g := NewGit()
	ctx := context.Background()

	t.Run("no upstream", func(t *testing.T) {
		dir := setupRepo(t)
		snap, err := g.Snapshot(ctx, dir)
		if err != nil {
			t.Fatalf("Snapshot: %v", err)
		}
		if snap.Branch != "main" {
			t.Errorf("Branch = %q, want main", snap.Branch)
		}
		if snap.HasUpstream || snap.Ahead != 0 || snap.Behind != 0 {
			t.Errorf("no-upstream snapshot = %+v, want no upstream counts", snap)
		}
	})

	t.Run("ahead of upstream with dirty worktree", func(t *testing.T) {
		dir := setupRepo(t)
		snapshotRun(t, dir, "config", "branch.main.remote", "origin")
		snapshotRun(t, dir, "config", "branch.main.merge", "refs/heads/main")
		// One commit ahead of the fabricated origin/main.
		if err := os.WriteFile(filepath.Join(dir, "g"), []byte("y"), 0o644); err != nil {
			t.Fatal(err)
		}
		snapshotRun(t, dir, "add", "g")
		snapshotRun(t, dir, "commit", "-m", "ahead")
		// Dirty state: staged, unstaged and untracked paths.
		if err := os.WriteFile(filepath.Join(dir, "staged"), []byte("s"), 0o644); err != nil {
			t.Fatal(err)
		}
		snapshotRun(t, dir, "add", "staged")
		if err := os.WriteFile(filepath.Join(dir, "f"), []byte("edit"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "untracked"), []byte("u"), 0o644); err != nil {
			t.Fatal(err)
		}

		snap, err := g.Snapshot(ctx, dir)
		if err != nil {
			t.Fatalf("Snapshot: %v", err)
		}
		if snap.Branch != "main" || !snap.HasUpstream {
			t.Errorf("branch/upstream = %+v, want main with upstream counts", snap)
		}
		if snap.Ahead != 1 || snap.Behind != 0 {
			t.Errorf("ahead/behind = %d/%d, want 1/0", snap.Ahead, snap.Behind)
		}
		if snap.Staged != 1 || snap.Unstaged != 1 || snap.Untracked != 1 {
			t.Errorf("worktree = %+v, want 1 staged, 1 unstaged, 1 untracked", snap.WorkTree)
		}
	})

	t.Run("detached HEAD", func(t *testing.T) {
		dir := setupRepo(t)
		snapshotRun(t, dir, "checkout", "--detach")
		snap, err := g.Snapshot(ctx, dir)
		if err != nil {
			t.Fatalf("Snapshot: %v", err)
		}
		// Parity with rev-parse --abbrev-ref HEAD, which reports "HEAD".
		if snap.Branch != "HEAD" {
			t.Errorf("detached Branch = %q, want HEAD", snap.Branch)
		}
		if snap.HasUpstream {
			t.Error("detached HEAD reports an upstream")
		}
	})
}

// TestParsePorcelainV2Entries covers entry kinds the fixture repos don't
// produce: renames (2), unmerged conflicts (u, counted as both staged and
// unstaged like the v1 parser) and ignored (!) lines.
func TestParsePorcelainV2Entries(t *testing.T) {
	out := "# branch.oid deadbeef\n" +
		"# branch.head main\n" +
		"# branch.upstream origin/main\n" +
		"# branch.ab +2 -3\n" +
		"1 .M N... 100644 100644 100644 aaaa bbbb modified.txt\n" +
		"2 R. N... 100644 100644 100644 aaaa bbbb R100 new.txt\told.txt\n" +
		"u UU N... 100644 100644 100644 100644 aaaa bbbb cccc conflict.txt\n" +
		"? untracked.txt\n" +
		"! ignored.txt\n"

	snap := parsePorcelainV2(out)
	if snap.Branch != "main" || !snap.HasUpstream {
		t.Errorf("header parse = %+v", snap)
	}
	if snap.Ahead != 2 || snap.Behind != 3 {
		t.Errorf("ahead/behind = %d/%d, want 2/3", snap.Ahead, snap.Behind)
	}
	// Staged: rename + conflict; unstaged: modified + conflict.
	if snap.Staged != 2 || snap.Unstaged != 2 || snap.Untracked != 1 {
		t.Errorf("worktree = %+v, want 2 staged, 2 unstaged, 1 untracked", snap.WorkTree)
	}
}
