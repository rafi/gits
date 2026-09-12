package git

import (
	"context"
	"errors"
	"fmt"
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

// TestRemoteBranches proves remote-tracking refs list as "<remote>/<branch>"
// names, and that a symbolic remote HEAD keeps its "origin/HEAD" name instead
// of being abbreviated to "origin".
func TestRemoteBranches(t *testing.T) {
	g, _ := NewGit()
	ctx := context.Background()
	dir := setupRepo(t)
	got, err := g.RemoteBranches(ctx, dir)
	if err != nil {
		t.Fatalf("RemoteBranches: %v", err)
	}
	if len(got) != 1 || got[0] != "origin/main" {
		t.Errorf("RemoteBranches = %v, want [origin/main]", got)
	}

	if out, err := exec.CommandContext(ctx, "git", "-C", dir,
		"symbolic-ref", "refs/remotes/origin/HEAD",
		"refs/remotes/origin/main").CombinedOutput(); err != nil {
		t.Fatalf("symbolic-ref: %v\n%s", err, out)
	}
	got, err = g.RemoteBranches(ctx, dir)
	if err != nil {
		t.Fatalf("RemoteBranches: %v", err)
	}
	want := map[string]bool{"origin/HEAD": true, "origin/main": true}
	if len(got) != len(want) {
		t.Fatalf("RemoteBranches = %v, want keys %v", got, want)
	}
	for _, b := range got {
		if !want[b] {
			t.Errorf("unexpected remote branch %q in %v", b, got)
		}
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

// TestAllBranches proves the checkout candidate list merges local branches
// with remote-only ones: locals first, remote prefix stripped, symbolic
// origin/HEAD dropped, and no duplicate for branches that exist both
// locally and remotely.
func TestAllBranches(t *testing.T) {
	g, _ := NewGit()
	ctx := context.Background()
	dir := setupRepo(t)
	run := func(args ...string) {
		t.Helper()
		if out, err := exec.CommandContext(ctx, "git",
			append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	head, err := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	sha := string(head[:len(head)-1])
	run("update-ref", "refs/remotes/origin/remote-only", sha)
	run("symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")

	got, err := g.AllBranches(ctx, dir)
	if err != nil {
		t.Fatalf("AllBranches: %v", err)
	}
	want := []string{"feature", "main", "remote-only"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("AllBranches = %v, want %v", got, want)
	}
}

// TestCheckoutRemoteOnlyBranch proves selecting a remote-only name works:
// git's DWIM creates a local tracking branch at the remote revision.
func TestCheckoutRemoteOnlyBranch(t *testing.T) {
	g, _ := NewGit()
	ctx := context.Background()
	dir := setupRepo(t)
	head, err := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	sha := string(head[:len(head)-1])
	if out, err := exec.CommandContext(ctx, "git", "-C", dir,
		"update-ref", "refs/remotes/origin/remote-only", sha).CombinedOutput(); err != nil {
		t.Fatalf("update-ref: %v\n%s", err, out)
	}

	if err := g.Checkout(ctx, dir, "remote-only"); err != nil {
		t.Fatalf("Checkout(remote-only): %v", err)
	}
	cur, err := g.CurrentBranch(ctx, dir)
	if err != nil {
		t.Fatalf("CurrentBranch: %v", err)
	}
	if cur != "remote-only" {
		t.Errorf("CurrentBranch = %q, want remote-only", cur)
	}
}

// TestUpstreamBranch covers error fidelity: a genuine missing upstream maps
// to ErrNoUpstream, a configured upstream resolves, and unrelated failures
// (context cancellation) must NOT masquerade as "no upstream".
func TestUpstreamBranch(t *testing.T) {
	g, _ := NewGit()
	ctx := context.Background()
	dir := setupRepo(t)
	run := func(args ...string) {
		t.Helper()
		if out, err := exec.CommandContext(ctx, "git",
			append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	t.Run("no upstream configured", func(t *testing.T) {
		if _, err := g.UpstreamBranch(ctx, dir); !errors.Is(err, ErrNoUpstream) {
			t.Errorf("UpstreamBranch = %v, want ErrNoUpstream", err)
		}
	})

	t.Run("cancelled context is not ErrNoUpstream", func(t *testing.T) {
		cctx, cancel := context.WithCancel(ctx)
		cancel()
		_, err := g.UpstreamBranch(cctx, dir)
		if err == nil {
			t.Fatal("UpstreamBranch with cancelled ctx succeeded, want error")
		}
		if errors.Is(err, ErrNoUpstream) {
			t.Errorf("cancellation misreported as ErrNoUpstream: %v", err)
		}
		if !errors.Is(err, context.Canceled) {
			t.Errorf("error %v does not wrap context.Canceled", err)
		}
	})

	t.Run("upstream configured", func(t *testing.T) {
		run("config", "branch.main.remote", "origin")
		run("config", "branch.main.merge", "refs/heads/main")
		got, err := g.UpstreamBranch(ctx, dir)
		if err != nil {
			t.Fatalf("UpstreamBranch: %v", err)
		}
		if got != "origin/main" {
			t.Errorf("UpstreamBranch = %q, want origin/main", got)
		}
	})
}

// TestFallbackRef proves the no-upstream comparison ref is resolved from the
// repo's actual remotes — preferring origin, falling back to any remote with
// a matching branch — instead of hardcoding "origin".
func TestFallbackRef(t *testing.T) {
	g, _ := NewGit()
	ctx := context.Background()
	dir := setupRepo(t)
	run := func(args ...string) {
		t.Helper()
		if out, err := exec.CommandContext(ctx, "git",
			append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	head, err := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	sha := string(head[:len(head)-1])
	run("remote", "add", "fork", dir)
	run("update-ref", "refs/remotes/fork/feature", sha)

	if got := g.FallbackRef(ctx, dir, "main"); got != "origin/main" {
		t.Errorf("FallbackRef(main) = %q, want origin/main", got)
	}
	if got := g.FallbackRef(ctx, dir, "feature"); got != "fork/feature" {
		t.Errorf("FallbackRef(feature) = %q, want fork/feature", got)
	}
	if got := g.FallbackRef(ctx, dir, "nope"); got != "" {
		t.Errorf("FallbackRef(nope) = %q, want empty", got)
	}
	// Detached HEAD: no remote HEAD ref yet, so nothing to compare against.
	if got := g.FallbackRef(ctx, dir, "HEAD"); got != "" {
		t.Errorf("FallbackRef(HEAD) = %q, want empty", got)
	}
	// Once the remote's HEAD ref exists, a detached checkout compares
	// against the remote default branch (parity with the pre-Snapshot path).
	run("symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	if got := g.FallbackRef(ctx, dir, "HEAD"); got != "origin/HEAD" {
		t.Errorf("FallbackRef(HEAD, remote HEAD present) = %q, want origin/HEAD", got)
	}

	// origin preferred when both remotes have the branch.
	run("update-ref", "refs/remotes/origin/feature", sha)
	if got := g.FallbackRef(ctx, dir, "feature"); got != "origin/feature" {
		t.Errorf("FallbackRef(feature, both remotes) = %q, want origin/feature", got)
	}
}
