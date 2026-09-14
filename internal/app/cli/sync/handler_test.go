package sync

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/app/cli/clitest"
)

// Every test here drives ExecSync — the command's real entry point — naming
// its projects explicitly. ExecSync never selects interactively, but the
// fixture is provider-backed, so recordCache also stands in for the warm
// repository cache that spares the loader a provider fetch.
//
// The narration is asserted on as whole lines. A test buffer is not a
// terminal, so clitest strips what the theme colored and the assertions see
// the words alone.

// recordCache reports every project as already cached — so the loader never
// contacts a provider — and records the projects it was asked to flush, which
// is what `sync` is for and therefore what these tests assert.
type recordCache struct{ flushed []string }

func (*recordCache) Get(string, *domain.Project) (bool, error) { return true, nil }
func (*recordCache) Save(string, domain.Project) error         { return nil }

func (c *recordCache) Flush(project domain.Project) error {
	c.flushed = append(c.flushed, project.Name)
	return nil
}

// syncDeps builds dependencies over two projects: one provider-backed and so
// cacheable, one that lists its repositories itself and has nothing to cache.
// A Provider Source needs its search term to be cacheable at all, which is
// also what lets recordCache answer for it.
func syncDeps(t *testing.T) (*clitest.Deps, *recordCache) {
	t.Helper()
	cache := &recordCache{}
	deps := clitest.New(t, clitest.FakeGit{})
	deps.Cache = cache
	deps.Projects = domain.ProjectListKeyed{
		"acme": {
			Name:   "acme",
			Source: &domain.ProviderSource{Type: "github", Search: "acme"},
			Repos:  []domain.Repository{{Name: "api", Src: "git@github.com:acme/api.git"}},
		},
		"local": clitest.NewProject(t, clitest.Cloned("tools")),
	}
	return deps, cache
}

// TestExecSyncNarratesNamedProject covers `gits sync acme`: the named
// project's cache is flushed, and its line names the project, the Provider
// Source and search term it was refreshed from, the remote entity ID that
// source resolved to, and how many repositories came back. The project the
// argument did not name is neither flushed nor narrated.
func TestExecSyncNarratesNamedProject(t *testing.T) {
	t.Parallel()

	deps, cache := syncDeps(t)

	if err := ExecSync([]string{"acme"}, deps.RuntimeCLI); err != nil {
		t.Fatalf("ExecSync(acme) error = %v, want nil", err)
	}

	if want := []string{"acme"}; !slices.Equal(cache.flushed, want) {
		t.Errorf("flushed %v, want %v", cache.flushed, want)
	}
	want := "[1/1] acme [github:acme] flushed · 1 repository\n"
	if deps.Result() != want {
		t.Errorf("Result Output = %q, want exactly %q", deps.Result(), want)
	}
}

// TestExecSyncVisitsOnlyRemoteProjects covers `gits sync` with no arguments:
// the provider-backed project is refreshed, and the one that lists its own
// repositories is passed over entirely. Only a remote project has a cached
// copy that can fall out of date; a local one is read from disk on every
// command, so there is nothing about it to refresh.
func TestExecSyncVisitsOnlyRemoteProjects(t *testing.T) {
	t.Parallel()

	deps, cache := syncDeps(t)

	if err := ExecSync(nil, deps.RuntimeCLI); err != nil {
		t.Fatalf("ExecSync() error = %v, want nil", err)
	}

	if want := []string{"acme"}; !slices.Equal(cache.flushed, want) {
		t.Errorf("flushed %v, want only the cacheable project %v", cache.flushed, want)
	}
	want := "[1/1] acme [github:acme] flushed · 1 repository\n"
	if deps.Result() != want {
		t.Errorf("Result Output = %q, want exactly %q", deps.Result(), want)
	}
}

// TestExecSyncSummaryIsDiagnostic proves the closing summary is about the run
// rather than part of it: it goes to Diagnostic Output, so the per-project
// lines a script reads from Result Output are not followed by a total.
func TestExecSyncSummaryIsDiagnostic(t *testing.T) {
	t.Parallel()

	deps, _ := syncDeps(t)

	if err := ExecSync(nil, deps.RuntimeCLI); err != nil {
		t.Fatalf("ExecSync() error = %v, want nil", err)
	}

	want := "Synchronized 1 project, 1 repository.\n"
	if got := deps.Diagnostic(); got != want {
		t.Errorf("Diagnostic Output = %q, want %q", got, want)
	}
	if strings.Contains(deps.Result(), "Synchronized") {
		t.Errorf("Result Output = %q, want the summary kept off it", deps.Result())
	}
}

// TestExecSyncUnknownProject proves syncing a non-existent project warns
// instead of silently exiting zero, and that it warns before anything is
// dropped or narrated: a typo costs the user nothing.
func TestExecSyncUnknownProject(t *testing.T) {
	t.Parallel()

	deps, cache := syncDeps(t)

	err := ExecSync([]string{"bogus"}, deps.RuntimeCLI)
	if err == nil {
		t.Fatal("ExecSync(bogus) = nil, want a warning")
	}
	if _, ok := errors.AsType[*domain.Warning](err); !ok {
		t.Errorf("ExecSync(bogus) error = %T (%v), want *domain.Warning", err, err)
	}
	if len(cache.flushed) > 0 {
		t.Errorf("flushed %v, want nothing cleaned for an unknown project", cache.flushed)
	}
	if got := deps.Result(); got != "" {
		t.Errorf("Result Output = %q, want empty", got)
	}
}

// treeCache answers every project from cache the way a real warm cache does:
// by populating the project it is handed, here with a Sub-project below the
// root.
type treeCache struct{ recordCache }

func (*treeCache) Get(_ string, project *domain.Project) (bool, error) {
	project.Repos = []domain.Repository{{Name: "api", Src: "git@github.com:acme/api.git"}}
	project.SubProjects = []domain.Project{{
		Name:  "tools",
		Repos: []domain.Repository{{Name: "cli", Src: "git@github.com:acme/cli.git"}},
	}}
	return true, nil
}

// TestExecSyncReportsPathAndWholeTree proves the line reports where the
// repositories landed, shortened with ~, and counts the whole tree — a
// GitLab group's subgroups are repositories of the project, and a count that
// stopped at the root would understate it.
func TestExecSyncReportsPathAndWholeTree(t *testing.T) {
	t.Parallel()

	deps, _ := syncDeps(t)
	deps.Cache = &treeCache{}
	deps.Projects["acme"] = domain.Project{
		Name:   "acme",
		Path:   clitest.HomeDir + "/code/acme",
		Source: &domain.ProviderSource{Type: "github", Search: "acme"},
	}

	if err := ExecSync([]string{"acme"}, deps.RuntimeCLI); err != nil {
		t.Fatalf("ExecSync(acme) error = %v, want nil", err)
	}

	want := "[1/1] acme [github:acme] flushed · ~/code/acme · 2 repositories\n"
	if deps.Result() != want {
		t.Errorf("Result Output = %q, want exactly %q", deps.Result(), want)
	}
}

// TestExecSyncFailureNamesTheProject proves a run that fails partway is
// legible: the project it died on has already been named on Result Output,
// so the error is attributable, and the summary is withheld because nothing
// was synchronized in full.
func TestExecSyncFailureNamesTheProject(t *testing.T) {
	t.Parallel()

	deps, _ := syncDeps(t)
	deps.Cache = &failingCache{}

	err := ExecSync([]string{"acme"}, deps.RuntimeCLI)
	if err == nil {
		t.Fatal("ExecSync(acme) = nil, want the flush failure")
	}
	if !strings.Contains(deps.Result(), "acme") {
		t.Errorf("Result Output = %q, want it to name the project that failed", deps.Result())
	}
	if strings.Contains(deps.Diagnostic(), "Synchronized") {
		t.Errorf("Diagnostic Output = %q, want no summary for a failed run", deps.Diagnostic())
	}
}

// failingCache fails the flush, standing in for an unwritable cache dir.
type failingCache struct{ recordCache }

func (*failingCache) Flush(domain.Project) error { return errors.New("disk on fire") }

// TestExecSyncNamedLocalProjectWarns proves naming a project that has nothing
// to sync says so rather than exiting zero having done nothing. A filesystem
// project is read from disk on every command, so there is no cached copy of
// it to refresh, and silence would read as success.
func TestExecSyncNamedLocalProjectWarns(t *testing.T) {
	t.Parallel()

	deps, cache := syncDeps(t)

	err := ExecSync([]string{"local"}, deps.RuntimeCLI)
	if err == nil {
		t.Fatal("ExecSync(local) = nil, want a warning")
	}
	if _, ok := errors.AsType[*domain.Warning](err); !ok {
		t.Errorf("ExecSync(local) error = %T (%v), want *domain.Warning", err, err)
	}
	if len(cache.flushed) > 0 {
		t.Errorf("flushed %v, want nothing for a project with no cache", cache.flushed)
	}
	if got := deps.Result(); got != "" {
		t.Errorf("Result Output = %q, want empty", got)
	}
}
