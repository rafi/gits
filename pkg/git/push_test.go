package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestPushOptionsValidate covers the mutual exclusion ADR-0002 requires: the
// three ref-selecting flags may not be combined, in any pairing, while every
// other flag composes freely. gits is stricter than git here on purpose —
// git accepts --all with its --branches synonym.
func TestPushOptionsValidate(t *testing.T) {
	tests := []struct {
		name    string
		opts    PushOptions
		wantErr bool
	}{
		{"empty", PushOptions{}, false},
		{"all", PushOptions{All: true}, false},
		{"branches", PushOptions{Branches: true}, false},
		{"tags", PushOptions{Tags: true}, false},
		{"all+branches", PushOptions{All: true, Branches: true}, true},
		{"all+tags", PushOptions{All: true, Tags: true}, true},
		{"branches+tags", PushOptions{Branches: true, Tags: true}, true},
		{"all three", PushOptions{All: true, Branches: true, Tags: true}, true},
		{"every other flag", PushOptions{
			FollowTags: true, Atomic: true, Prune: true, DryRun: true,
		}, false},
		{"tags with the rest", PushOptions{
			Tags: true, FollowTags: true, Atomic: true, Prune: true, DryRun: true,
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.opts.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && !strings.Contains(err.Error(), "--all") {
				t.Errorf("Validate() = %q, want it to name the conflicting flags", err)
			}
		})
	}
}

// TestPushOptionsSelectsRefs proves the predicate the handler suspends its
// Upstream lookup on covers exactly the three ref-selecting flags.
func TestPushOptionsSelectsRefs(t *testing.T) {
	tests := []struct {
		name string
		opts PushOptions
		want bool
	}{
		{"empty", PushOptions{}, false},
		{"all", PushOptions{All: true}, true},
		{"branches", PushOptions{Branches: true}, true},
		{"tags", PushOptions{Tags: true}, true},
		{"only passthrough", PushOptions{
			FollowTags: true, Atomic: true, Prune: true, DryRun: true,
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.opts.SelectsRefs(); got != tt.want {
				t.Errorf("SelectsRefs() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestPushOptionsArgs checks every passthrough flag reaches git as itself, and
// that no flag from the excluded set can be produced.
func TestPushOptionsArgs(t *testing.T) {
	if got := (PushOptions{}).args(); len(got) != 0 {
		t.Errorf("zero PushOptions produced %v, want no flags", got)
	}

	all := PushOptions{
		All: true, Branches: true, Tags: true,
		FollowTags: true, Atomic: true, Prune: true, DryRun: true,
	}
	got := all.args()
	want := []string{
		"--all", "--branches", "--tags",
		"--follow-tags", "--atomic", "--prune", "--dry-run",
	}
	if !slices.Equal(got, want) {
		t.Errorf("args() = %v, want %v", got, want)
	}

	// AC-8: the excluded flags have no representation at all.
	for _, banned := range []string{
		"--force", "--force-with-lease", "-u", "--set-upstream", "--mirror",
		"-q", "--quiet",
	} {
		if slices.Contains(got, banned) {
			t.Errorf("args() produced %q, which gits must never reach", banned)
		}
	}
}

// TestSplitUpstream covers the remote/branch split, including the two shapes
// that name no remote: a bare local upstream and an empty ref.
func TestSplitUpstream(t *testing.T) {
	tests := []struct {
		upstream   string
		wantRemote string
		wantBranch string
		wantOK     bool
	}{
		{"origin/main", "origin", "main", true},
		{"upstream/release/v2", "upstream", "release/v2", true},
		{"main", "", "", false},
		{"", "", "", false},
		{"origin/", "", "", false},
		{"/main", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.upstream, func(t *testing.T) {
			remote, branch, ok := SplitUpstream(tt.upstream)
			if remote != tt.wantRemote || branch != tt.wantBranch || ok != tt.wantOK {
				t.Errorf("SplitUpstream(%q) = (%q, %q, %v), want (%q, %q, %v)",
					tt.upstream, remote, branch, ok,
					tt.wantRemote, tt.wantBranch, tt.wantOK)
			}
		})
	}
}

// setupPushRepo builds a work tree with a bare remote at "origin", with main
// tracking origin/main and an unpushed commit waiting on it.
func setupPushRepo(t *testing.T) (work, bare string) {
	t.Helper()
	requireGit(t)
	ctx := context.Background()
	root := t.TempDir()
	bare = filepath.Join(root, "bare.git")
	work = filepath.Join(root, "work")

	run := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	for _, dir := range []string{bare, work} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	run(bare, "init", "--bare", "-b", "main")
	run(work, "init", "-b", "main")
	write := func(content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(work, "f"), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("one")
	run(work, "add", "f")
	run(work, "commit", "-m", "one")
	run(work, "remote", "add", "origin", bare)
	run(work, "push", "-u", "origin", "main")
	run(work, "tag", "v1")
	write("two")
	run(work, "commit", "-am", "two")
	return work, bare
}

// remoteHead returns the bare repository's commit for ref, or "" when absent.
func remoteHead(t *testing.T, bare, ref string) string {
	t.Helper()
	out, err := exec.CommandContext(
		context.Background(), "git", "-C", bare, "rev-parse", "--verify", "-q", ref,
	).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// localHead returns the work tree's commit for ref.
func localHead(t *testing.T, work, ref string) string {
	t.Helper()
	out, err := exec.CommandContext(
		context.Background(), "git", "-C", work, "rev-parse", ref,
	).Output()
	if err != nil {
		t.Fatalf("rev-parse %s: %v", ref, err)
	}
	return strings.TrimSpace(string(out))
}

// TestPushToUpstream is the default mode end to end: an explicit remote and
// refspec move the remote branch to the local commit.
func TestPushToUpstream(t *testing.T) {
	g := NewGit()
	work, bare := setupPushRepo(t)

	target := PushTarget{Remote: "origin", Refspec: "main:main"}
	if _, err := g.Push(context.Background(), work, target, PushOptions{}); err != nil {
		t.Fatalf("Push() = %v, want nil", err)
	}
	if got, want := remoteHead(t, bare, "refs/heads/main"), localHead(t, work, "main"); got != want {
		t.Errorf("remote main = %q, want %q", got, want)
	}
}

// TestPushDryRunLeavesRemoteUntouched proves --dry-run is delegated to git
// rather than approximated: the call succeeds and the remote does not move.
func TestPushDryRunLeavesRemoteUntouched(t *testing.T) {
	g := NewGit()
	work, bare := setupPushRepo(t)
	before := remoteHead(t, bare, "refs/heads/main")

	target := PushTarget{Remote: "origin", Refspec: "main:main"}
	if _, err := g.Push(context.Background(), work, target, PushOptions{DryRun: true}); err != nil {
		t.Fatalf("Push(--dry-run) = %v, want nil", err)
	}
	if got := remoteHead(t, bare, "refs/heads/main"); got != before {
		t.Errorf("remote main = %q after --dry-run, want it unchanged at %q", got, before)
	}
	if before == localHead(t, work, "main") {
		t.Fatal("fixture is not proving anything: remote already at the local commit")
	}
}

// TestPushTagsWithoutTarget covers --tags mode: a zero PushTarget lets git
// resolve the destination, and only tags travel.
func TestPushTagsWithoutTarget(t *testing.T) {
	g := NewGit()
	work, bare := setupPushRepo(t)
	before := remoteHead(t, bare, "refs/heads/main")

	if _, err := g.Push(context.Background(), work, PushTarget{}, PushOptions{Tags: true}); err != nil {
		t.Fatalf("Push(--tags) = %v, want nil", err)
	}
	if remoteHead(t, bare, "refs/tags/v1") == "" {
		t.Error("tag v1 did not reach the remote")
	}
	if got := remoteHead(t, bare, "refs/heads/main"); got != before {
		t.Errorf("remote main = %q, want --tags to leave branches alone at %q", got, before)
	}
}

// TestPushRejectsInvalidOptions proves the primitive refuses a combination the
// CLI is supposed to have caught, so no caller can fan a bad one out.
func TestPushRejectsInvalidOptions(t *testing.T) {
	g := NewGit()
	opts := PushOptions{All: true, Tags: true}
	if _, err := g.Push(context.Background(), t.TempDir(), PushTarget{}, opts); err == nil {
		t.Fatal("Push with --all and --tags = nil, want an error before git runs")
	}
}

// TestPushRejectsFlagLikeRemote proves the remote is separated from flags: a
// "-"-prefixed remote must reach git as a remote name (and be rejected as
// unknown), never be parsed as a git flag.
func TestPushRejectsFlagLikeRemote(t *testing.T) {
	g := NewGit()
	work, _ := setupPushRepo(t)

	target := PushTarget{Remote: "--mirror", Refspec: "main:main"}
	if _, err := g.Push(context.Background(), work, target, PushOptions{}); err == nil {
		t.Fatal(`Push with flag-like remote "--mirror" = nil, want error`)
	}
}

// TestPushFailureSurfacesGitMessage proves a rejected push (here: a remote
// that does not exist) fails with git's own words rather than "exit status 1".
func TestPushFailureSurfacesGitMessage(t *testing.T) {
	g := NewGit()
	work, _ := setupPushRepo(t)

	target := PushTarget{Remote: "nowhere", Refspec: "main:main"}
	_, err := g.Push(context.Background(), work, target, PushOptions{})
	if err == nil {
		t.Fatal("Push to an unknown remote = nil, want an error")
	}
	if !strings.Contains(err.Error(), "nowhere") {
		t.Errorf("Push() = %q, want git's message naming the remote", err)
	}
}
