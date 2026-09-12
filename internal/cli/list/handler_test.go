package list

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cli/clitest"
)

// Every test here drives ExecList — the command's real entry point — with
// explicit project arguments, so no renderer is constructed by hand and
// argument parsing never reaches the interactive finder.
//
// The fixture needs no filesystem and no git client: a repository only
// reaches Repo State `ok` if the loader stats a real path, while `remote-only`
// and `error` are decided from configuration alone. The one thing standing in
// the way is the provider fetch a remote Provider Source would trigger, and
// hitCache stands in for the repository cache that spares a real run the same
// fetch.

// hitCache reports every project as already cached, so the loader never
// contacts a provider and leaves the fixture's repositories exactly as
// declared — the same repositories a warm cache would have restored.
type hitCache struct{}

func (hitCache) Get(string, *domain.Project) (bool, error) { return true, nil }
func (hitCache) Save(string, domain.Project) error         { return nil }
func (hitCache) Flush(domain.Project) error                { return nil }

// testDeps builds the shared runtime dependencies with both output
// destinations captured, a nil git client — nothing here may reach one — and
// the fixture project.
func testDeps(t *testing.T) *clitest.Deps {
	t.Helper()
	deps := clitest.New(t, nil)
	deps.Cache = hitCache{}
	deps.Projects = domain.ProjectListKeyed{"acme": fixtureProject()}
	return deps
}

// fixtureProject is one provider-backed project with no Project Path: its
// repositories therefore have no local home, which is `remote-only` for those
// carrying a Repo Src and `error` for the one whose relative Repo Dir has no
// Project Path to resolve against.
//
// The Provider Source needs its search term for more than realism: a source
// without one is not cacheable, so hitCache would never be consulted and the
// loader would go to the network.
func fixtureProject() domain.Project {
	return domain.Project{
		Source: &domain.ProviderSource{Type: "github", Search: "acme"},
		Repos: []domain.Repository{
			{Name: "api", Src: "git@github.com:acme/api.git"},
			{Name: "web", Dir: "web"},
		},
		SubProjects: []domain.Project{{
			Name: "tools",
			Repos: []domain.Repository{
				{Name: "cli", Src: "git@github.com:acme/cli.git"},
			},
			SubProjects: []domain.Project{{
				Name: "deep",
				Repos: []domain.Repository{
					{Name: "core", Src: "git@github.com:acme/core.git"},
				},
			}},
		}},
	}
}

// wantReason is the Reason the fixture's `error` repository carries.
const wantReason = "relative `dir:` \"web\" requires the project to set `path:`"

// TestExecListWritesResultOutput proves every output format writes what it
// renders to Result Output and nothing to Diagnostic Output. A format still
// naming a process stream would leave the captured Result Output empty.
func TestExecListWritesResultOutput(t *testing.T) {
	t.Parallel()

	for _, format := range []string{"name", "tree", "table", "wide", "json"} {
		t.Run(format, func(t *testing.T) {
			t.Parallel()

			deps := testDeps(t)
			if err := ExecList(format, []string{"acme"}, deps.RuntimeCLI); err != nil {
				t.Fatalf("ExecList(%q) error = %v, want nil", format, err)
			}
			if deps.Result() == "" {
				t.Errorf("Result Output is empty, want the rendered %s output", format)
			}
			if got := deps.Diagnostic(); got != "" {
				t.Errorf("Diagnostic Output = %q, want empty", got)
			}
		})
	}
}

// TestExecListNameRepos covers `gits list -o name acme`: one repository name
// per line and nothing else, which is what a shell pipeline consumes.
func TestExecListNameRepos(t *testing.T) {
	t.Parallel()

	deps := testDeps(t)
	if err := ExecList("name", []string{"acme"}, deps.RuntimeCLI); err != nil {
		t.Fatalf("ExecList error = %v, want nil", err)
	}

	want := "api\ntools/cli\ntools/deep/core\nweb\n"
	if got := deps.Result(); got != want {
		t.Errorf("Result Output = %q, want %q", got, want)
	}
}

// countingGit answers IsRepo by stat and counts every Remote call, so a test
// can prove which output formats resolve Repo Src and which never touch git.
type countingGit struct {
	clitest.FakeGit

	remote      string
	mu          sync.Mutex
	remoteCalls int
}

func (g *countingGit) Remote(context.Context, string) (string, error) {
	g.mu.Lock()
	g.remoteCalls++
	g.mu.Unlock()
	return g.remote, nil
}

// IsRepo answers by stat rather than the embedded always-true, so the
// filesystem walk finds the fixture's repositories instead of stopping at the
// project root.
func (g *countingGit) IsRepo(_ context.Context, path string) bool {
	_, err := os.Stat(filepath.Join(path, ".git"))
	return err == nil
}

func (g *countingGit) calls() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.remoteCalls
}

// fsProject is a filesystem-backed project whose repositories are cloned on
// disk, so they classify `ok` with no Repo Src — the only shape whose Src is
// resolved lazily.
func fsProject(t *testing.T) domain.Project {
	t.Helper()
	root := t.TempDir()
	for _, name := range []string{"api", "web"} {
		if err := os.MkdirAll(filepath.Join(root, name, ".git"), 0o750); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
	}
	return domain.Project{Path: root, Source: &domain.ProviderSource{Type: "filesystem", Search: root}}
}

// TestExecListSrcResolutionIsLazy pins ticket 11: `list -o name` and `-o tree`
// never print Repo Src and so must issue zero `git ls-remote` calls, while the
// table, wide and JSON formats display it and resolve it once per repository.
func TestExecListSrcResolutionIsLazy(t *testing.T) {
	t.Parallel()

	t.Run("name and tree issue no remote calls", func(t *testing.T) {
		t.Parallel()

		for _, format := range []string{"name", "tree"} {
			g := &countingGit{remote: "git@x:a/b.git"}
			deps := clitest.New(t, g)
			deps.Cache = hitCache{}
			deps.Projects = domain.ProjectListKeyed{"acme": fsProject(t)}

			if err := ExecList(format, []string{"acme"}, deps.RuntimeCLI); err != nil {
				t.Fatalf("ExecList(%q) error = %v, want nil", format, err)
			}
			if got := g.calls(); got != 0 {
				t.Errorf("%s format: Remote calls = %d, want 0", format, got)
			}
		}
	})

	t.Run("table, wide and json resolve src once per repo", func(t *testing.T) {
		t.Parallel()

		for _, format := range []string{"table", "wide", "json"} {
			g := &countingGit{remote: "git@x:a/b.git"}
			deps := clitest.New(t, g)
			deps.Cache = hitCache{}
			deps.Projects = domain.ProjectListKeyed{"acme": fsProject(t)}

			if err := ExecList(format, []string{"acme"}, deps.RuntimeCLI); err != nil {
				t.Fatalf("ExecList(%q) error = %v, want nil", format, err)
			}
			if got := g.calls(); got != 2 {
				t.Errorf("%s format: Remote calls = %d, want 2 (one per repo)", format, got)
			}
		}
	})
}

// TestExecListNameProjects covers `gits list -o name` with no project named:
// the name format switches to project names, sorted.
func TestExecListNameProjects(t *testing.T) {
	t.Parallel()

	deps := testDeps(t)
	deps.Projects["beta"] = fixtureProject()

	if err := ExecList("name", nil, deps.RuntimeCLI); err != nil {
		t.Fatalf("ExecList error = %v, want nil", err)
	}

	want := "acme\nbeta\n"
	if got := deps.Result(); got != want {
		t.Errorf("Result Output = %q, want %q", got, want)
	}
}

// TestExecListTree covers the tree format nesting a Sub-project beneath its
// parent, rather than listing it as a sibling. The fixture nests twice, so
// the assertion reads named nodes at both depths.
//
// Repository nodes are not named here: a project with no Project Path has
// nothing to name its repositories relative to, so their leaves render as an
// empty path. The nesting of Sub-projects, not the leaf text, is what this
// test pins.
func TestExecListTree(t *testing.T) {
	t.Parallel()

	deps := testDeps(t)
	if err := ExecList("tree", []string{"acme"}, deps.RuntimeCLI); err != nil {
		t.Fatalf("ExecList error = %v, want nil", err)
	}

	got := strings.Split(strings.TrimRight(deps.Result(), "\n"), "\n")
	if len(got) < 3 {
		t.Fatalf("Result Output = %q, want a tree of at least three lines", deps.Result())
	}
	if !strings.HasPrefix(got[0], "acme") {
		t.Errorf("root line = %q, want it to start with the project name", got[0])
	}
	if !strings.HasPrefix(got[1], "├── ") || !strings.Contains(got[1], "tools") {
		t.Errorf("second line = %q, want the sub-project branched off the root", got[1])
	}
	// The Sub-project of that Sub-project sits one level deeper still, which
	// the continuation bar of its parent's branch marks.
	if !strings.HasPrefix(got[2], "│   ├── ") || !strings.Contains(got[2], "deep") {
		t.Errorf("third line = %q, want the nested sub-project beneath its parent", got[2])
	}
}

// TestExecListTable covers the table format's columns, and that an `error`
// repository reaches the user with its Reason in the source column — that
// column is where a repository with no Repo Src explains itself.
func TestExecListTable(t *testing.T) {
	t.Parallel()

	deps := testDeps(t)
	if err := ExecList("table", []string{"acme"}, deps.RuntimeCLI); err != nil {
		t.Fatalf("ExecList error = %v, want nil", err)
	}

	got := deps.Result()
	for _, header := range []string{"TITLE", "STATE", "SOURCE"} {
		if !strings.Contains(got, header) {
			t.Errorf("Result Output = %q, want the %s column", got, header)
		}
	}
	if !strings.Contains(got, "remote-only") {
		t.Errorf("Result Output = %q, want the remote-only state", got)
	}
	if !strings.Contains(got, wantReason) {
		t.Errorf("Result Output = %q, want the Reason of the error repository", got)
	}
}

// TestExecListWide covers the wide format: the same table with the path
// column appended.
func TestExecListWide(t *testing.T) {
	t.Parallel()

	deps := testDeps(t)
	if err := ExecList("wide", []string{"acme"}, deps.RuntimeCLI); err != nil {
		t.Fatalf("ExecList error = %v, want nil", err)
	}

	got := deps.Result()
	if !strings.Contains(got, "PATH") {
		t.Errorf("Result Output = %q, want the PATH column", got)
	}
	if !strings.Contains(got, "TITLE") {
		t.Errorf("Result Output = %q, want the table columns as well", got)
	}
}

// TestExecListJSON covers the envelope ADR-0001 fixed: one document of
// projects keyed by name, each repository carrying its Repo State, and a
// Reason on the `error` one.
func TestExecListJSON(t *testing.T) {
	t.Parallel()

	deps := testDeps(t)
	if err := ExecList("json", []string{"acme"}, deps.RuntimeCLI); err != nil {
		t.Fatalf("ExecList error = %v, want nil", err)
	}

	raw := deps.Result()
	if got := strings.Count(raw, "\n"); got != 1 || !strings.HasSuffix(raw, "\n") {
		t.Errorf("Result Output = %q, want one newline-terminated line", raw)
	}

	// Decoded structurally rather than through the envelope types, so the
	// wire contract is checked against something other than itself.
	var env map[string]struct {
		Name  string `json:"name"`
		Repos []struct {
			Name   string `json:"name"`
			State  string `json:"state"`
			Reason string `json:"reason"`
		} `json:"repos"`
		SubProjects []struct {
			Name  string `json:"name"`
			Repos []struct {
				Name  string `json:"name"`
				State string `json:"state"`
			} `json:"repos"`
		} `json:"subprojects"`
	}
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		t.Fatalf("unmarshal Result Output: %v", err)
	}

	proj, ok := env["acme"]
	if !ok {
		t.Fatalf("envelope = %+v, want the project keyed by its name", env)
	}
	if len(proj.Repos) != 2 {
		t.Fatalf("repos = %+v, want two", proj.Repos)
	}
	if proj.Repos[0].State != string(domain.RepoStateRemoteOnly) {
		t.Errorf("api state = %q, want %q", proj.Repos[0].State, domain.RepoStateRemoteOnly)
	}
	if proj.Repos[0].Reason != "" {
		t.Errorf("api reason = %q, want none outside the error state", proj.Repos[0].Reason)
	}
	if proj.Repos[1].State != string(domain.RepoStateError) {
		t.Errorf("web state = %q, want %q", proj.Repos[1].State, domain.RepoStateError)
	}
	if proj.Repos[1].Reason != wantReason {
		t.Errorf("web reason = %q, want %q", proj.Repos[1].Reason, wantReason)
	}
	if len(proj.SubProjects) != 1 || proj.SubProjects[0].Name != "tools" {
		t.Fatalf("subprojects = %+v, want the sub-project nested", proj.SubProjects)
	}
	if got := proj.SubProjects[0].Repos; len(got) != 1 || got[0].Name != "cli" {
		t.Errorf("sub-project repos = %+v, want its own repository", got)
	}
}

// TestExecListDeterministicOrder covers the table and tree formats rendering a
// multi-project map in a stable, sorted order rather than the map's random
// iteration order. Three projects named out of order must come out
// alphabetically, the same way every run.
func TestExecListDeterministicOrder(t *testing.T) {
	t.Parallel()

	build := func(t *testing.T) *clitest.Deps {
		t.Helper()
		deps := clitest.New(t, nil)
		deps.Cache = hitCache{}
		deps.Projects = domain.ProjectListKeyed{
			"charlie": {Source: &domain.ProviderSource{Type: "github", Search: "c"},
				Repos: []domain.Repository{{Name: "c", Src: "git@github.com:c/c.git"}}},
			"alpha": {Source: &domain.ProviderSource{Type: "github", Search: "a"},
				Repos: []domain.Repository{{Name: "a", Src: "git@github.com:a/a.git"}}},
			"bravo": {Source: &domain.ProviderSource{Type: "github", Search: "b"},
				Repos: []domain.Repository{{Name: "b", Src: "git@github.com:b/b.git"}}},
		}
		return deps
	}

	for _, format := range []string{"table", "wide", "tree"} {
		t.Run(format, func(t *testing.T) {
			t.Parallel()

			// Rendered repeatedly: a map's iteration order is randomized per
			// range, so a run that only happened to sort would drift here.
			var first string
			for i := range 8 {
				deps := build(t)
				if err := ExecList(format, []string{"alpha", "bravo", "charlie"}, deps.RuntimeCLI); err != nil {
					t.Fatalf("ExecList(%q) error = %v, want nil", format, err)
				}
				got := deps.Result()

				a := strings.Index(got, "alpha")
				b := strings.Index(got, "bravo")
				c := strings.Index(got, "charlie")
				if a < 0 || b < 0 || c < 0 {
					t.Fatalf("Result Output = %q, want all three project names", got)
				}
				if a >= b || b >= c {
					t.Errorf("order = alpha@%d bravo@%d charlie@%d, want alphabetical", a, b, c)
				}
				if i == 0 {
					first = got
				} else if got != first {
					t.Errorf("run %d differs from run 0:\n%q\nvs\n%q", i, got, first)
				}
			}
		})
	}
}

// TestExecListUnknownFormat covers an unrecognized format being rejected
// before any project is loaded. The fixture's Provider Source is invalid, so
// a load would fail with its own error — the format error arriving instead is
// what says nothing was loaded.
func TestExecListUnknownFormat(t *testing.T) {
	t.Parallel()

	deps := testDeps(t)
	deps.Projects = domain.ProjectListKeyed{"acme": {
		Source: &domain.ProviderSource{Type: "nope"},
		Repos:  []domain.Repository{{Name: "api"}},
	}}

	err := ExecList("yaml", []string{"acme"}, deps.RuntimeCLI)
	if err == nil {
		t.Fatalf("ExecList(%q) error = nil, want a rejected format", "yaml")
	}
	if !strings.Contains(err.Error(), "unknown output format") {
		t.Errorf("error = %v, want it to name the unknown output format", err)
	}
	if got := deps.Result(); got != "" {
		t.Errorf("Result Output = %q, want nothing rendered", got)
	}
	if got := deps.Diagnostic(); got != "" {
		t.Errorf("Diagnostic Output = %q, want nothing rendered", got)
	}

	// The same dependencies with a known format do reach the loader and fail
	// there, which is what makes the assertion above meaningful.
	if err := ExecList("name", []string{"acme"}, deps.RuntimeCLI); err == nil ||
		strings.Contains(err.Error(), "unknown output format") {
		t.Errorf("ExecList(\"name\") error = %v, want the load to have been attempted", err)
	}
}

// TestExecListTracingStaysOutOfOutput proves the split ticket 15 established
// and ticket 16 preserved: debug tracing goes to the logger, never to Result
// Output and never to Diagnostic Output. `list` over a filesystem source
// traces its provider search, so the records exist to be misplaced.
func TestExecListTracingStaysOutOfOutput(t *testing.T) {
	t.Parallel()

	deps := clitest.New(t, &countingGit{remote: "git@x:a/b.git"})
	deps.Projects = domain.ProjectListKeyed{"acme": fsProject(t)}

	if err := ExecList("name", []string{"acme"}, deps.RuntimeCLI); err != nil {
		t.Fatalf("ExecList error = %v, want nil", err)
	}

	if !strings.Contains(deps.Trace(), "searching for repos on disk") {
		t.Fatalf("trace = %q, want the provider search recorded", deps.Trace())
	}
	for name, out := range map[string]string{
		"Result Output":     deps.Result(),
		"Diagnostic Output": deps.Diagnostic(),
	} {
		if strings.Contains(out, "level=") || strings.Contains(out, "msg=") {
			t.Errorf("%s = %q, want no log records in it", name, out)
		}
	}
}
