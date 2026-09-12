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
	"github.com/rafi/gits/pkg/git"
)

// Every test here drives ExecPull — the command's real entry point — with
// explicit arguments, so argument parsing, the choice between walking a whole
// Project and acting on a single Repository, the error epilogue and the exit
// code all run for real, and the interactive finder is never reached.

// fakeGit implements the three calls pullRepo makes and records the
// repositories it pulled, since which were pulled — and which were passed over
// — is itself what several tests assert. Everything else is inherited from
// clitest.FakeGit and panics if reached.
type fakeGit struct {
	clitest.FakeGit
	branch      string
	branchErr   error
	upstream    string
	upstreamErr error
	pullOut     string
	pullErr     error

	mu     sync.Mutex
	pulled []string
}

func (f *fakeGit) CurrentBranch(context.Context, string) (string, error) {
	return f.branch, f.branchErr
}

func (f *fakeGit) UpstreamBranch(context.Context, string) (string, error) {
	return f.upstream, f.upstreamErr
}

func (f *fakeGit) Pull(_ context.Context, path string) (string, error) {
	f.mu.Lock()
	f.pulled = append(f.pulled, filepath.Base(path))
	f.mu.Unlock()
	return f.pullOut, f.pullErr
}

// Pulled returns the name of every repository Pull was called for, in the
// order the walker reached them.
func (f *fakeGit) Pulled() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.pulled)
}

// tracking is a fake whose repositories all sit on a branch with an Upstream,
// which is the only condition pull works under.
func tracking() *fakeGit {
	return &fakeGit{branch: "main", upstream: "origin/main", pullOut: "up to date"}
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
	g := tracking()
	deps := clitest.New(t, g).WithProject("acme", clitest.Cloned("api"), clitest.Cloned("web"))

	if err := ExecPull([]string{"acme"}, deps.RuntimeCLI); err != nil {
		t.Fatalf("ExecPull error = %v, want nil", err)
	}

	got := deps.Result()
	for _, want := range []string{"acme", "api", "web", "main <- origin/main", "up to date"} {
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

// TestExecPullNoUpstream covers a branch with no Upstream: unlike push, which
// passes over it as a warning, pull has nowhere to pull from and counts it as a
// failure. Both ways of saying so — the sentinel and an empty answer — are
// covered, and the single-Repository path is driven so the repository's own
// error is returned rather than the run's summary.
func TestExecPullNoUpstream(t *testing.T) {
	for _, tc := range []struct {
		name string
		git  *fakeGit
	}{
		{"sentinel", &fakeGit{branch: "main", upstreamErr: git.ErrNoUpstream}},
		{"empty upstream", &fakeGit{branch: "main", upstream: ""}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			deps := clitest.New(t, tc.git).WithProject("acme", clitest.Cloned("api"))

			err := ExecPull([]string{"acme", "api"}, deps.RuntimeCLI)
			if err == nil {
				t.Fatal("ExecPull error = nil, want a branch with no Upstream to fail")
			}
			if !errors.Is(err, git.ErrNoUpstream) {
				t.Errorf("ExecPull error = %v, want it to wrap ErrNoUpstream", err)
			}
			if len(tc.git.Pulled()) != 0 {
				t.Errorf("pulled %v, want nothing pulled", tc.git.Pulled())
			}
			if got := deps.Result(); !strings.Contains(got, "no upstream") {
				t.Errorf("Result Output = %q, want the repository's line to explain", got)
			}
		})
	}
}

// TestExecPullUpstreamFailureNotMislabeled covers an UpstreamBranch failure that
// is not ErrNoUpstream (e.g. cancellation): it must surface as itself, not as
// the misleading "no upstream tracking branch found".
func TestExecPullUpstreamFailureNotMislabeled(t *testing.T) {
	g := &fakeGit{branch: "main", upstreamErr: context.Canceled}
	deps := clitest.New(t, g).WithProject("acme", clitest.Cloned("api"))

	err := ExecPull([]string{"acme", "api"}, deps.RuntimeCLI)
	if err == nil {
		t.Fatal("ExecPull error = nil, want the cancelled lookup to fail")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("ExecPull error = %v, want it to wrap context.Canceled", err)
	}
	if strings.Contains(err.Error(), "no upstream") {
		t.Errorf("ExecPull error = %v, want the cancellation not mislabeled as a skip", err)
	}
}

// TestExecPullBranchError covers a failure resolving the current branch: it is
// counted, and nothing is pulled.
func TestExecPullBranchError(t *testing.T) {
	g := &fakeGit{branchErr: errors.New("detached HEAD")}
	deps := clitest.New(t, g).WithProject("acme", clitest.Cloned("api"))

	err := ExecPull([]string{"acme", "api"}, deps.RuntimeCLI)
	if err == nil {
		t.Fatal("ExecPull error = nil, want an unresolvable branch to fail")
	}
	if !strings.Contains(err.Error(), "detached HEAD") {
		t.Errorf("ExecPull error = %v, want git's message", err)
	}
	if len(g.Pulled()) != 0 {
		t.Errorf("pulled %v, want nothing pulled", g.Pulled())
	}
}
