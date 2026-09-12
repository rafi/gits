package sync

import (
	"errors"
	"slices"
	"testing"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cli/clitest"
	"github.com/rafi/gits/internal/types"
)

// Every test here drives ExecSync — the command's real entry point — naming
// its projects explicitly. ExecSync never selects interactively, but the
// fixture is provider-backed, so recordCache also stands in for the warm
// repository cache that spares the loader a provider fetch.

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

// TestExecSyncCleansNamedProject covers `gits sync acme`: the named project's
// cache is flushed and reported on Result Output, and the project the argument
// did not name is left alone.
func TestExecSyncCleansNamedProject(t *testing.T) {
	deps, cache := syncDeps(t)

	if err := ExecSync([]string{"acme"}, deps.RuntimeCLI); err != nil {
		t.Fatalf("ExecSync(acme) error = %v, want nil", err)
	}

	if want := []string{"acme"}; !slices.Equal(cache.flushed, want) {
		t.Errorf("flushed %v, want %v", cache.flushed, want)
	}
	if want := "Cleaned \"acme\" project cache.\n"; deps.Result() != want {
		t.Errorf("Result Output = %q, want exactly %q", deps.Result(), want)
	}
	if got := deps.Diagnostic(); got != "" {
		t.Errorf("Diagnostic Output = %q, want empty", got)
	}
}

// TestExecSyncPassesOverUncacheableProject covers `gits sync` with no
// arguments: every cacheable project is cleaned, and a project with no
// Provider Source has no cache to clean and is passed over silently.
func TestExecSyncPassesOverUncacheableProject(t *testing.T) {
	deps, cache := syncDeps(t)

	if err := ExecSync(nil, deps.RuntimeCLI); err != nil {
		t.Fatalf("ExecSync() error = %v, want nil", err)
	}

	if want := []string{"acme"}; !slices.Equal(cache.flushed, want) {
		t.Errorf("flushed %v, want only the cacheable project %v", cache.flushed, want)
	}
	if want := "Cleaned \"acme\" project cache.\n"; deps.Result() != want {
		t.Errorf("Result Output = %q, want exactly %q", deps.Result(), want)
	}
}

// TestExecSyncUnknownProject proves syncing a non-existent project warns
// instead of silently exiting zero.
func TestExecSyncUnknownProject(t *testing.T) {
	deps, cache := syncDeps(t)

	err := ExecSync([]string{"bogus"}, deps.RuntimeCLI)
	if err == nil {
		t.Fatal("ExecSync(bogus) = nil, want a warning")
	}
	var warn *types.Warning
	if !errors.As(err, &warn) {
		t.Errorf("ExecSync(bogus) error = %T (%v), want *types.Warning", err, err)
	}
	if len(cache.flushed) > 0 {
		t.Errorf("flushed %v, want nothing cleaned for an unknown project", cache.flushed)
	}
	if got := deps.Result(); got != "" {
		t.Errorf("Result Output = %q, want empty", got)
	}
}
