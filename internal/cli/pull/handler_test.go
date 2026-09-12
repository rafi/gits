package pull

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/rafi/gits/internal/cli/clitest"
	"github.com/rafi/gits/internal/git"
)

// Every test here drives ExecPull — the command's real entry point — with
// explicit arguments, so argument parsing, the choice between walking a whole
// Project and acting on a single Repository, the error epilogue and the exit
// code all run for real, and the interactive finder is never reached.

// fakeGit implements the two calls pullRepo makes and records the repositories
// it pulled, since which were pulled — and which were passed over — is itself
// what several tests assert. Everything else is inherited from clitest.FakeGit
// and panics if reached.
type fakeGit struct {
	clitest.FakeGit

	head    git.HeadRef
	headErr error
	pullOut string
	pullErr error

	mu     sync.Mutex
	pulled []string
}

func (f *fakeGit) HeadUpstream(context.Context, string) (git.HeadRef, error) {
	return f.head, f.headErr
}

func (f *fakeGit) Pull(_ context.Context, path string) (string, error) {
	f.mu.Lock()
	f.pulled = append(f.pulled, filepath.Base(path))
	f.mu.Unlock()
	return f.pullOut, f.pullErr
}

// Pulled returns the name of every repository Pull was called for, in the
// order the Traversal reached them.
func (f *fakeGit) Pulled() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.pulled)
}

// tracking is a fake whose repositories all sit on a branch with an Upstream
// that resolves, which is the only condition pull works under.
func tracking() *fakeGit {
	return &fakeGit{
		head:    git.HeadRef{Branch: "main", Upstream: "origin/main"},
		pullOut: "up to date",
	}
}

// TestExecPullProject covers `gits pull acme`: every repository of the project
// is pulled and its result reaches Result Output under the project title,
// naming the branch and the Upstream it was pulled from.
//
// Diagnostic Output being empty is the second assertion: a run with nothing to
// report emits no epilogue, and the live progress reporter — handed a buffer
// rather than a terminal — emits nothing at all, so no ANSI can reach any
// assertion in this package.
func TestExecPullProject(t *testing.T) {
	t.Parallel()

	g := tracking()
	deps := clitest.New(t, g).WithProject("acme", clitest.Cloned("api"), clitest.Cloned("web"))

	if err := ExecPull([]string{"acme"}, deps.RuntimeCLI); err != nil {
		t.Fatalf("ExecPull error = %v, want nil", err)
	}

	got := deps.Result()
	for _, want := range []string{"api", "web", "main <- origin/main", "up to date"} {
		if !strings.Contains(got, want) {
			t.Errorf("Result Output = %q, want it to contain %q", got, want)
		}
	}
	if want := []string{"api", "web"}; !slices.Equal(g.Pulled(), want) {
		t.Errorf("pulled %v, want %v", g.Pulled(), want)
	}
	if got := deps.Diagnostic(); got != "" {
		t.Errorf("Diagnostic Output = %q, want empty", got)
	}
}

// TestExecPullSingleRepo covers `gits pull acme api`: the second argument
// selects one repository, and only that one is pulled and rendered — without
// the project title the whole-project path prints.
func TestExecPullSingleRepo(t *testing.T) {
	t.Parallel()

	g := tracking()
	deps := clitest.New(t, g).WithProject("acme", clitest.Cloned("api"), clitest.Cloned("web"))

	if err := ExecPull([]string{"acme", "api"}, deps.RuntimeCLI); err != nil {
		t.Fatalf("ExecPull error = %v, want nil", err)
	}

	got := deps.Result()
	if !strings.Contains(got, "api") || !strings.Contains(got, "up to date") {
		t.Errorf("Result Output = %q, want the selected repository's result", got)
	}
	if strings.Contains(got, "web") {
		t.Errorf("Result Output = %q, want nothing about the other repository", got)
	}
	if want := []string{"api"}; !slices.Equal(g.Pulled(), want) {
		t.Errorf("pulled %v, want %v", g.Pulled(), want)
	}
}

// TestExecPullSkipsNonOKRepositories covers the state guard: a repository that
// is not cloned, and one whose configuration is defective, are both passed over
// without a pull and both count toward the exit code. The defective one reports
// the Reason it was classified with, not a generic message.
func TestExecPullSkipsNonOKRepositories(t *testing.T) {
	t.Parallel()

	g := tracking()
	deps := clitest.New(t, g).WithProject("acme",
		clitest.Cloned("api"), clitest.NotCloned("gone"), clitest.Broken("bad"))

	err := ExecPull([]string{"acme"}, deps.RuntimeCLI)
	if err == nil {
		t.Fatal("ExecPull error = nil, want the skipped repositories to fail the run")
	}

	if want := []string{"api"}; !slices.Equal(g.Pulled(), want) {
		t.Errorf("pulled %v, want only the cloned repository %v", g.Pulled(), want)
	}
	for _, want := range []string{"not cloned", clitest.BrokenReason} {
		if got := deps.Result(); !strings.Contains(got, want) {
			t.Errorf("Result Output = %q, want it to contain %q", got, want)
		}
		if got := deps.Diagnostic(); !strings.Contains(got, want) {
			t.Errorf("Diagnostic Output = %q, want the epilogue to contain %q", got, want)
		}
	}
	if got := deps.Diagnostic(); !strings.Contains(got, "2 errors:") {
		t.Errorf("Diagnostic Output = %q, want both skips counted in the epilogue", got)
	}
}

// TestExecPullFailureReportsEpilogue covers a repository whose pull fails:
// git's message reaches Result Output on the repository's line and Diagnostic
// Output in the error epilogue, and the run reports failure.
func TestExecPullFailureReportsEpilogue(t *testing.T) {
	t.Parallel()

	g := tracking()
	g.pullErr = errors.New("would clobber local changes")
	deps := clitest.New(t, g).WithProject("acme", clitest.Cloned("api"))

	err := ExecPull([]string{"acme"}, deps.RuntimeCLI)
	if err == nil {
		t.Fatal("ExecPull error = nil, want the failed repository to fail the run")
	}
	if !strings.Contains(err.Error(), "completed with errors") {
		t.Errorf("ExecPull error = %v, want the run reported as failed", err)
	}
	if got := deps.Result(); !strings.Contains(got, "would clobber local changes") {
		t.Errorf("Result Output = %q, want git's message on the repository line", got)
	}
	got := deps.Diagnostic()
	if !strings.Contains(got, "1 error:") ||
		!strings.Contains(got, "would clobber local changes") {
		t.Errorf("Diagnostic Output = %q, want the error epilogue", got)
	}
}

// TestExecPullUnpullableIsSkipped covers the two branches pull has nowhere to
// pull from: one with no Upstream at all, and one whose Upstream is gone —
// merged and cleaned up on the Remote. Both are reported on the repository's
// line, both leave the exit code alone, and the gone one names the Upstream
// that went away so the two read differently.
//
// The no-Upstream case is the deliberate change from the hard error pull used
// to raise, which is what makes it agree with push about an unpushable branch.
func TestExecPullUnpullableIsSkipped(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		head git.HeadRef
		want []string
	}{
		{
			name: "no upstream",
			head: git.HeadRef{Branch: "main"},
			want: []string{"skipped", git.ErrNoUpstream.Error()},
		},
		{
			name: "gone upstream",
			head: git.HeadRef{Branch: "feat-b", Upstream: "origin/feat-b", Gone: true},
			want: []string{"skipped", "origin/feat-b", git.ErrUpstreamGone.Error()},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			g := &fakeGit{head: tc.head}
			deps := clitest.New(t, g).WithProject("acme", clitest.Cloned("api"))

			if err := ExecPull([]string{"acme"}, deps.RuntimeCLI); err != nil {
				t.Fatalf("ExecPull error = %v, want a skipped repository not to fail the run", err)
			}
			if len(g.Pulled()) != 0 {
				t.Errorf("pulled %v, want nothing pulled", g.Pulled())
			}
			got := deps.Result()
			for _, want := range tc.want {
				if !strings.Contains(got, want) {
					t.Errorf("Result Output = %q, want the line to contain %q", got, want)
				}
			}
			// Nothing git says about the condition reaches the user: the state
			// is read from a query that answers it, not from a failure.
			for _, banned := range []string{"fatal:", "@{upstream}", "ambiguous argument"} {
				if strings.Contains(got, banned) {
					t.Errorf("Result Output = %q, want no raw git error text (%q)", got, banned)
				}
			}
			if got := deps.Diagnostic(); got != "" {
				t.Errorf("Diagnostic Output = %q, want a warning to leave the epilogue empty", got)
			}

			// Naming the repository changes nothing: the skip is a warning on
			// its line, and the run still succeeds.
			single := clitest.New(t, &fakeGit{head: tc.head}).
				WithProject("acme", clitest.Cloned("api"))
			if err := ExecPull([]string{"acme", "api"}, single.RuntimeCLI); err != nil {
				t.Errorf("ExecPull error = %v, want the single-repository skip not to fail the run", err)
			}
		})
	}
}

// TestExecPullSkipsReadDifferently covers the two skips being distinguishable:
// the gone one names its Upstream, and the no-Upstream one claims nothing about
// an Upstream it does not have.
func TestExecPullSkipsReadDifferently(t *testing.T) {
	t.Parallel()

	line := func(t *testing.T, head git.HeadRef) string {
		t.Helper()
		deps := clitest.New(t, &fakeGit{head: head}).
			WithProject("acme", clitest.Cloned("api"))
		if err := ExecPull([]string{"acme"}, deps.RuntimeCLI); err != nil {
			t.Fatalf("ExecPull error = %v, want nil", err)
		}
		return deps.Result()
	}

	none := line(t, git.HeadRef{Branch: "main"})
	gone := line(t, git.HeadRef{Branch: "feat-b", Upstream: "origin/feat-b", Gone: true})
	if none == gone {
		t.Errorf("both skips rendered %q, want the two conditions to read apart", none)
	}
	if strings.Contains(none, "origin/") {
		t.Errorf("Result Output = %q, want no Upstream named for a branch without one", none)
	}
}

// TestExecPullHeadFailure covers the Upstream lookup itself failing (e.g.
// cancellation): it is reported as itself on the line and in the epilogue, and
// counts toward the exit code rather than being downgraded to one of the
// documented skips.
func TestExecPullHeadFailure(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{"canceled", context.Canceled, context.Canceled.Error()},
		{"git failure", errors.New("not a git repository"), "not a git repository"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			g := &fakeGit{headErr: tc.err}
			deps := clitest.New(t, g).WithProject("acme", clitest.Cloned("api"))

			err := ExecPull([]string{"acme", "api"}, deps.RuntimeCLI)
			if err == nil {
				t.Fatal("ExecPull error = nil, want the failed lookup to fail the run")
			}
			if got := deps.Diagnostic(); !strings.Contains(got, "1 error:") ||
				!strings.Contains(got, tc.want) {
				t.Errorf("Diagnostic Output = %q, want the epilogue to carry %q", got, tc.want)
			}
			if got := deps.Result(); !strings.Contains(got, tc.want) {
				t.Errorf("Result Output = %q, want the line to carry %q", got, tc.want)
			}
			if len(g.Pulled()) != 0 {
				t.Errorf("pulled %v, want nothing pulled", g.Pulled())
			}
		})
	}
}
