package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestDiffCounts builds two divergent branches and asserts Diff tallies the
// left (<, ahead) and right (>, behind) revisions from `rev-list --left-right`.
func TestDiffCounts(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git executable not available: %v", err)
	}
	ctx := context.Background()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	commit := func(name string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
		run("add", ".")
		run("commit", "-m", name)
	}

	run("init", "-b", "main")
	commit("base")
	run("branch", "feature")
	// main gains 2 commits, feature gains 3 — diverging from the shared base.
	commit("m1")
	commit("m2")
	run("checkout", "feature")
	commit("f1")
	commit("f2")
	commit("f3")

	g, err := NewGit()
	if err != nil {
		t.Fatalf("NewGit: %v", err)
	}

	t.Run("divergent branches", func(t *testing.T) {
		ahead, behind, err := g.Diff(ctx, dir, "feature", "main")
		if err != nil {
			t.Fatalf("Diff: %v", err)
		}
		if ahead != 3 || behind != 2 {
			t.Errorf("Diff(feature, main) = (%d, %d), want (3, 2)", ahead, behind)
		}
	})

	t.Run("equal refs", func(t *testing.T) {
		ahead, behind, err := g.Diff(ctx, dir, "main", "main")
		if err != nil {
			t.Fatalf("Diff: %v", err)
		}
		if ahead != 0 || behind != 0 {
			t.Errorf("Diff(main, main) = (%d, %d), want (0, 0)", ahead, behind)
		}
	})
}
