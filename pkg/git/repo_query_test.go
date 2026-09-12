package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// setupRepo creates a throwaway git repo with one commit on branch "main", a
// second branch "feature", a remote "origin", and a remote-tracking ref
// origin/main. It returns the repo path. The test is skipped if git is absent.
func setupRepo(t *testing.T) string {
	t.Helper()
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
	run("init", "-b", "main")
	if err := os.WriteFile(filepath.Join(dir, "f"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "f")
	run("commit", "-m", "init")
	run("branch", "feature")
	run("remote", "add", "origin", dir)
	// Fabricate a remote-tracking ref without a network fetch.
	out, err := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	run("update-ref", "refs/remotes/origin/main", string(out[:len(out)-1]))
	return dir
}

func TestIsRepo(t *testing.T) {
	g, _ := NewGit()
	ctx := context.Background()
	dir := setupRepo(t)
	if !g.IsRepo(ctx, dir) {
		t.Errorf("IsRepo(%q) = false, want true", dir)
	}
	if g.IsRepo(ctx, t.TempDir()) {
		t.Error("IsRepo on a non-repo dir = true, want false")
	}
}

func TestBranches(t *testing.T) {
	g, _ := NewGit()
	dir := setupRepo(t)
	got, err := g.Branches(context.Background(), dir)
	if err != nil {
		t.Fatalf("Branches: %v", err)
	}
	want := map[string]bool{"main": true, "feature": true}
	if len(got) != len(want) {
		t.Fatalf("Branches = %v, want keys %v", got, want)
	}
	for _, b := range got {
		if !want[b] {
			t.Errorf("unexpected branch %q in %v", b, got)
		}
	}
}

func TestRemotes(t *testing.T) {
	g, _ := NewGit()
	got, err := g.Remotes(context.Background(), setupRepo(t))
	if err != nil {
		t.Fatalf("Remotes: %v", err)
	}
	if len(got) != 1 || got[0] != "origin" {
		t.Errorf("Remotes = %v, want [origin]", got)
	}
}

func TestHasRemoteBranch(t *testing.T) {
	g, _ := NewGit()
	ctx := context.Background()
	dir := setupRepo(t)
	if !g.HasRemoteBranch(ctx, dir, "origin", "main") {
		t.Error("HasRemoteBranch(origin, main) = false, want true")
	}
	if g.HasRemoteBranch(ctx, dir, "origin", "nope") {
		t.Error("HasRemoteBranch(origin, nope) = true, want false")
	}
}

func TestCheckout(t *testing.T) {
	g, _ := NewGit()
	ctx := context.Background()
	dir := setupRepo(t)
	if err := g.Checkout(ctx, dir, "feature"); err != nil {
		t.Fatalf("Checkout: %v", err)
	}
	cur, err := g.CurrentBranch(ctx, dir)
	if err != nil {
		t.Fatalf("CurrentBranch: %v", err)
	}
	if cur != "feature" {
		t.Errorf("after Checkout, CurrentBranch = %q, want feature", cur)
	}
}
