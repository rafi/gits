package sync

import (
	"slices"
	"testing"

	"github.com/rafi/gits/domain"
	coreruntime "github.com/rafi/gits/internal/runtime"
)

// names renders repositories as their namespaced names, for comparison.
func names(repos []domain.Repository) []string {
	out := []string{}
	for _, repo := range repos {
		out = append(out, repo.GetNameWithNamespace())
	}
	return out
}

// TestNewReposKeysByForgeID proves a repository the code forge still reports
// under the same ID is not new even if it was renamed, while one with an ID
// the cache never held is.
func TestNewReposKeysByForgeID(t *testing.T) {
	t.Parallel()

	source := &domain.ProviderSource{Type: "github", Search: "acme"}
	cached := domain.Project{Source: source, Repos: []domain.Repository{
		{ID: "1", Name: "api", Namespace: "acme"},
	}}
	loaded := domain.Project{Source: source, Repos: []domain.Repository{
		{ID: "1", Name: "api-v2", Namespace: "acme"},
		{ID: "2", Name: "widgets", Namespace: "acme"},
	}}

	previous := map[string]bool{}
	walkRepos(cached, source.UniqueKey(),
		func(repo domain.Repository) { previous[repoKey(repo)] = true })

	got := names(newRepos(loaded, previous))
	if want := []string{"acme/widgets"}; !slices.Equal(got, want) {
		t.Errorf("newRepos = %v, want %v", got, want)
	}
}

// TestNewReposSkipsSubProjectWithOwnSource proves a Sub-project discovered
// from a source of its own is left out of the comparison: it is cached
// apart from its parent, so all of its repositories would otherwise read as
// new. A Sub-project that inherits the parent's source is still compared.
func TestNewReposSkipsSubProjectWithOwnSource(t *testing.T) {
	t.Parallel()

	source := &domain.ProviderSource{Type: "gitlab", Search: "acme"}
	loaded := domain.Project{
		Source: source,
		Repos:  []domain.Repository{{Name: "api", Namespace: "acme"}},
		SubProjects: []domain.Project{
			{
				Name:   "inherited",
				Source: source,
				Repos:  []domain.Repository{{Name: "cli", Namespace: "acme/tools"}},
			},
			{
				Name:   "own",
				Source: &domain.ProviderSource{Type: "github", Search: "other"},
				Repos:  []domain.Repository{{Name: "lib", Namespace: "other"}},
			},
		},
	}

	previous := map[string]bool{"name:acme/api": true}
	got := names(newRepos(loaded, previous))
	if want := []string{"acme/tools/cli"}; !slices.Equal(got, want) {
		t.Errorf("newRepos = %v, want %v", got, want)
	}
}

// peekCache serves a fixed earlier entry from Peek.
type peekCache struct{ previous domain.Project }

func (peekCache) Get(string, *domain.Project) (bool, error) { return false, nil }
func (peekCache) Save(string, domain.Project) error         { return nil }
func (peekCache) Flush(domain.Project) error                { return nil }
func (c peekCache) Peek(string) (domain.Project, bool, error) {
	return c.previous, true, nil
}

// TestCachedReposIgnoredWhenCachingDisabled proves a cache file left from
// before caching was turned off is not compared with: how stale it is is
// unknown, so it would report long-standing repositories as new.
func TestCachedReposIgnoredWhenCachingDisabled(t *testing.T) {
	t.Parallel()

	off := false
	project := domain.Project{Source: &domain.ProviderSource{Type: "github", Search: "acme"}}
	rt := coreruntime.Runtime{
		Cache:    peekCache{previous: domain.Project{Repos: []domain.Repository{{Name: "api"}}}},
		Settings: domain.Settings{Cache: &off},
	}
	if got := cachedRepos(project, rt); got != nil {
		t.Errorf("cachedRepos with caching off = %v, want nil", got)
	}

	rt.Settings.Cache = nil
	if got := cachedRepos(project, rt); !got["name:api"] {
		t.Errorf("cachedRepos with caching on = %v, want it to hold api", got)
	}
}
