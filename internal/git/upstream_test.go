package git

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// setupUpstreamRepo builds a clone whose branches cover every Upstream case
// the ref walk must tell apart:
//
//   - main       tracks origin/main, which exists
//   - feat       tracks origin/feat, deleted on the remote and pruned here
//   - solo       has no upstream at all
//   - localtrack tracks the local main, so its upstream names no remote
//
// The gone state is constructed the way a user reaches it — configure an
// Upstream by checking the branch out, delete it on the remote, prune the
// remote-tracking ref — rather than by writing the config git would write.
func setupUpstreamRepo(t *testing.T) string {
	t.Helper()
	requireGit(t)

	origin := t.TempDir()
	snapshotRun(t, origin, "init", "-q", "-b", "main")
	snapshotRun(t, origin, "commit", "-q", "--allow-empty", "-m", "init")
	snapshotRun(t, origin, "checkout", "-q", "-b", "feat")
	snapshotRun(t, origin, "commit", "-q", "--allow-empty", "-m", "feat")
	snapshotRun(t, origin, "checkout", "-q", "main")

	parent := t.TempDir()
	snapshotRun(t, parent, "clone", "-q", origin, "clone")
	clone := filepath.Join(parent, "clone")

	snapshotRun(t, clone, "checkout", "-q", "feat")
	snapshotRun(t, origin, "branch", "-q", "-D", "feat")
	snapshotRun(t, clone, "fetch", "-q", "--prune")
	snapshotRun(t, clone, "checkout", "-q", "--no-track", "-b", "solo", "main")
	snapshotRun(t, clone, "branch", "-q", "--track", "localtrack", "main")

	return clone
}

// TestHeadUpstream proves one ref walk answers with the current branch and
// the state of its Upstream, against a real git.
//
//nolint:paralleltest // the subtests check branches out in one shared clone, so they run in sequence.
func TestHeadUpstream(t *testing.T) {
	g := NewGit()
	ctx := context.Background()
	dir := setupUpstreamRepo(t)

	tests := []struct {
		name   string
		branch string
		want   HeadRef
	}{
		{
			name:   "tracked upstream",
			branch: "main",
			want:   HeadRef{Branch: "main", Upstream: "origin/main"},
		},
		{
			name:   "gone upstream",
			branch: "feat",
			want:   HeadRef{Branch: "feat", Upstream: "origin/feat", Gone: true},
		},
		{
			name:   "no upstream",
			branch: "solo",
			want:   HeadRef{Branch: "solo"},
		},
		{
			// A branch tracking another local branch abbreviates to a bare
			// name: an Upstream that is not on a Remote, which is a case of
			// its own rather than a missing Upstream.
			name:   "upstream on no remote",
			branch: "localtrack",
			want:   HeadRef{Branch: "localtrack", Upstream: "main"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			snapshotRun(t, dir, "checkout", "-q", tc.branch)
			got, err := g.HeadUpstream(ctx, dir)
			if err != nil {
				t.Fatalf("HeadUpstream: %v", err)
			}
			if got != tc.want {
				t.Errorf("HeadUpstream = %+v, want %+v", got, tc.want)
			}
		})
	}

	// A failed ref walk keeps its own identity: nothing here reads a failure
	// as an Upstream state, so a canceled context surfaces as cancellation
	// rather than as "no upstream".
	t.Run("canceled context", func(t *testing.T) {
		cctx, cancel := context.WithCancel(ctx)
		cancel()
		if _, err := g.HeadUpstream(cctx, dir); !errors.Is(err, context.Canceled) {
			t.Errorf("HeadUpstream with canceled ctx = %v, want context.Canceled", err)
		}
	})

	t.Run("detached HEAD", func(t *testing.T) {
		snapshotRun(t, dir, "checkout", "-q", "--detach")
		got, err := g.HeadUpstream(ctx, dir)
		if err != nil {
			t.Fatalf("HeadUpstream: %v", err)
		}
		// Parity with CurrentBranch, which reports "HEAD" when detached.
		if got != (HeadRef{Branch: "HEAD"}) {
			t.Errorf("detached HeadUpstream = %+v, want branch HEAD alone", got)
		}
	})
}

// TestGoneUpstreamRendering pins the two undocumented facts this design rests
// on, against a real git: the ref walk renders a Gone Upstream literally
// "[gone]", and porcelain v2 emits the upstream header while omitting the
// ahead/behind one. A fake cannot falsify either, so only this test catches
// git changing its behavior.
func TestGoneUpstreamRendering(t *testing.T) {
	t.Parallel()

	dir := setupUpstreamRepo(t)
	snapshotRun(t, dir, "checkout", "-q", "feat")

	track := gitOutput(t, dir, "for-each-ref", "--format=%(upstream:track)",
		"refs/heads/feat")
	if track != goneTrack {
		t.Errorf("%%(upstream:track) = %q, want %q", track, goneTrack)
	}

	porcelain := gitOutput(t, dir, "status", "--porcelain=v2", "--branch")
	if !strings.Contains(porcelain, "# branch.upstream origin/feat") {
		t.Errorf("porcelain omits the upstream header:\n%s", porcelain)
	}
	if strings.Contains(porcelain, "# branch.ab ") {
		t.Errorf("porcelain emits ahead/behind for a gone upstream:\n%s", porcelain)
	}
}

// gitOutput runs git in dir and returns its trimmed stdout.
func gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), "git",
		append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return strings.TrimSpace(string(out))
}

// TestParseHeadUpstream pins the ref-walk parse against a canned fixture, so
// the four cases are covered without needing git.
func TestParseHeadUpstream(t *testing.T) {
	t.Parallel()

	const sep = "\x00"
	rows := []string{
		" " + sep + "main" + sep + "origin/main" + sep + "[behind 2]",
		"*" + sep + "feat" + sep + "origin/feat" + sep + goneTrack,
		" " + sep + "solo" + sep + "" + sep + "",
		" " + sep + "localtrack" + sep + "main" + sep + "",
	}

	got := parseHeadUpstream(strings.Join(rows, "\n"))
	want := HeadRef{Branch: "feat", Upstream: "origin/feat", Gone: true}
	if got != want {
		t.Errorf("parseHeadUpstream = %+v, want %+v", got, want)
	}

	// The marked row is picked wherever it sits, first row included.
	marked := parseHeadUpstream("*" + sep + "main" + sep + "origin/main" + sep + "")
	if marked != (HeadRef{Branch: "main", Upstream: "origin/main"}) {
		t.Errorf("parseHeadUpstream(first row marked) = %+v", marked)
	}

	// No row marked: a detached or unborn HEAD.
	detached := parseHeadUpstream(" " + sep + "main" + sep + "" + sep + "")
	if detached != (HeadRef{Branch: "HEAD"}) {
		t.Errorf("parseHeadUpstream(no marked row) = %+v, want branch HEAD", detached)
	}
	if empty := parseHeadUpstream(""); empty != (HeadRef{Branch: "HEAD"}) {
		t.Errorf("parseHeadUpstream(empty) = %+v, want branch HEAD", empty)
	}
}
