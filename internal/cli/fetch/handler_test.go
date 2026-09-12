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

	if err := ExecFetch("table", []string{"acme"}, deps.RuntimeCLI); err != nil {
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

	if err := ExecFetch("table", []string{"acme", "api"}, deps.RuntimeCLI); err != nil {
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

	err := ExecFetch("table", []string{"acme"}, deps.RuntimeCLI)
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

	err := ExecFetch("table", []string{"acme"}, deps.RuntimeCLI)
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

// TestExecFetchJSON covers `gits fetch -o json acme`: what fetch made of each
// repository nests under `fetch` as exactly one of output or error, and a
// repository the state guard turned back carries its state and no such
// object. Neither condition fails the run or prints an epilogue in this
// format. Fetch has no documented pass-over, so `skipped` never appears.
func TestExecFetchJSON(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		git  *fakeGit
		key  string
		want string
	}{
		{"fetched", &fakeGit{out: "up to date"}, "output", "up to date"},
		{"failed", &fakeGit{err: errors.New("network down")}, "error", "network down"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			deps := clitest.New(t, tc.git).WithProject("acme",
				clitest.Cloned("api"), clitest.NotCloned("gone"), clitest.Broken("bad"))

			if err := ExecFetch("json", []string{"acme"}, deps.RuntimeCLI); err != nil {
				t.Fatalf("ExecFetch error = %v, want nil: a repository's condition is data", err)
			}
			if got := deps.Diagnostic(); got != "" {
				t.Errorf("Diagnostic Output = %q, want no epilogue", got)
			}

			repos := deps.JSONRepos("acme")
			outcome, ok := repos["api"]["fetch"].(map[string]any)
			if !ok {
				t.Fatalf("api = %v, want the outcome under \"fetch\"", repos["api"])
			}
			// The body's text is what the table shows, which for fetch
			// carries the repository's path ahead of git's report whenever
			// the display path is not the whole of it.
			if got, _ := outcome[tc.key].(string); len(outcome) != 1 || !strings.HasSuffix(got, tc.want) {
				t.Errorf("api.fetch = %v, want only %q ending in %q", outcome, tc.key, tc.want)
			}
			for name, state := range map[string]string{"gone": "not-cloned", "bad": "error"} {
				if repos[name]["state"] != state {
					t.Errorf("%s.state = %v, want %s", name, repos[name]["state"], state)
				}
				if _, found := repos[name]["fetch"]; found {
					t.Errorf("%s = %v, want no outcome for a repository fetch never ran for",
						name, repos[name])
				}
			}
			if repos["bad"]["reason"] != clitest.BrokenReason {
				t.Errorf("bad.reason = %v, want the Reason it was classified with", repos["bad"]["reason"])
			}
		})
	}
}

// TestExecFetchUnknownFormat covers a format other than table or json being
// rejected before anything is loaded.
func TestExecFetchUnknownFormat(t *testing.T) {
	t.Parallel()

	g := &fakeGit{out: "up to date"}
	deps := clitest.New(t, g).WithProject("acme", clitest.Cloned("api"))

	err := ExecFetch("wide", []string{"acme"}, deps.RuntimeCLI)
	if err == nil || !strings.Contains(err.Error(), "unknown output format") {
		t.Fatalf("ExecFetch(\"wide\") error = %v, want the format rejected", err)
	}
	if len(g.Fetched()) != 0 {
		t.Errorf("fetched %v, want nothing reached before the format was checked", g.Fetched())
	}
	if got := deps.Result() + deps.Diagnostic(); got != "" {
		t.Errorf("output = %q, want nothing rendered", got)
	}
}
