package fetch

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/rafi/gits/internal/cli/clitest"
)

// Every test here drives ExecFetch — the command's real entry point — with
// explicit arguments, so argument parsing, the choice between walking a whole
// Project and acting on a single Repository, the error epilogue and the exit
// code all run for real, and the interactive finder is never reached.

// fakeGit implements the one call fetchRepo makes and records the repositories
// it was called for, since which were fetched — and which were passed over —
// is itself what several tests assert. Everything else is inherited from
// clitest.FakeGit and panics if reached.
type fakeGit struct {
	clitest.FakeGit

	out string
	err error

	mu      sync.Mutex
	fetched []string
}

func (f *fakeGit) Fetch(_ context.Context, path string) (string, error) {
	f.mu.Lock()
	f.fetched = append(f.fetched, filepath.Base(path))
	f.mu.Unlock()
	return f.out, f.err
}

// Fetched returns the name of every repository Fetch was called for, in the
// order the Traversal reached them.
func (f *fakeGit) Fetched() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.fetched)
}

// TestExecFetchProject covers `gits fetch acme`: every repository of the
// project is fetched and its result reaches Result Output under the project
// title.
//
// Diagnostic Output being empty is the second assertion: a run with nothing to
// report emits no epilogue, and the live progress reporter — handed a buffer
// rather than a terminal — emits nothing at all, so no ANSI can reach any
// assertion in this package.
func TestExecFetchProject(t *testing.T) {
	t.Parallel()

	g := &fakeGit{out: "up to date"}
	deps := clitest.New(t, g).WithProject("acme", clitest.Cloned("api"), clitest.Cloned("web"))

	if err := ExecFetch([]string{"acme"}, deps.RuntimeCLI); err != nil {
		t.Fatalf("ExecFetch error = %v, want nil", err)
	}

	got := deps.Result()
	for _, want := range []string{"api", "web", "up to date"} {
		if !strings.Contains(got, want) {
			t.Errorf("Result Output = %q, want it to contain %q", got, want)
		}
	}
	if want := []string{"api", "web"}; !slices.Equal(g.Fetched(), want) {
		t.Errorf("fetched %v, want %v", g.Fetched(), want)
	}
	if got := deps.Diagnostic(); got != "" {
		t.Errorf("Diagnostic Output = %q, want empty", got)
	}
}

// TestExecFetchSingleRepo covers `gits fetch acme api`: the second argument
// selects one repository, and only that one is fetched and rendered — without
// the project title the whole-project path prints.
func TestExecFetchSingleRepo(t *testing.T) {
	t.Parallel()

	g := &fakeGit{out: "up to date"}
	deps := clitest.New(t, g).WithProject("acme", clitest.Cloned("api"), clitest.Cloned("web"))

	if err := ExecFetch([]string{"acme", "api"}, deps.RuntimeCLI); err != nil {
		t.Fatalf("ExecFetch error = %v, want nil", err)
	}

	got := deps.Result()
	if !strings.Contains(got, "api") || !strings.Contains(got, "up to date") {
		t.Errorf("Result Output = %q, want the selected repository's result", got)
	}
	if strings.Contains(got, "web") {
		t.Errorf("Result Output = %q, want nothing about the other repository", got)
	}
	if want := []string{"api"}; !slices.Equal(g.Fetched(), want) {
		t.Errorf("fetched %v, want %v", g.Fetched(), want)
	}
}

// TestExecFetchSkipsNonOKRepositories covers the state guard: a repository that
// is not cloned, and one whose configuration is defective, are both passed over
// without a fetch and both count toward the exit code. The defective one
// reports the Reason it was classified with, not a generic message.
func TestExecFetchSkipsNonOKRepositories(t *testing.T) {
	t.Parallel()

	g := &fakeGit{out: "up to date"}
	deps := clitest.New(t, g).WithProject("acme",
		clitest.Cloned("api"), clitest.NotCloned("gone"), clitest.Broken("bad"))

	err := ExecFetch([]string{"acme"}, deps.RuntimeCLI)
	if err == nil {
		t.Fatal("ExecFetch error = nil, want the skipped repositories to fail the run")
	}

	if want := []string{"api"}; !slices.Equal(g.Fetched(), want) {
		t.Errorf("fetched %v, want only the cloned repository %v", g.Fetched(), want)
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

// TestExecFetchFailureReportsEpilogue covers a repository whose fetch fails:
// git's message reaches Result Output on the repository's line and Diagnostic
// Output in the error epilogue, and the run reports failure.
func TestExecFetchFailureReportsEpilogue(t *testing.T) {
	t.Parallel()

	g := &fakeGit{err: errors.New("network down")}
	deps := clitest.New(t, g).WithProject("acme", clitest.Cloned("api"))

	err := ExecFetch([]string{"acme"}, deps.RuntimeCLI)
	if err == nil {
		t.Fatal("ExecFetch error = nil, want the failed repository to fail the run")
	}
	if !strings.Contains(err.Error(), "completed with errors") {
		t.Errorf("ExecFetch error = %v, want the run reported as failed", err)
	}
	if got := deps.Result(); !strings.Contains(got, "network down") {
		t.Errorf("Result Output = %q, want git's message on the repository line", got)
	}
	got := deps.Diagnostic()
	if !strings.Contains(got, "1 error:") || !strings.Contains(got, "network down") {
		t.Errorf("Diagnostic Output = %q, want the error epilogue", got)
	}
}
