// Package sync refreshes each Project's cached repository list from its
// Provider Source. It writes nothing: the work is exposed one Project at a
// time — Targets says which Projects a run will visit, One visits a single
// Project — so the caller drives the loop and narrates it.
package sync

import (
	"fmt"
	"slices"
	"strings"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/infra/providers"
	"github.com/rafi/gits/internal/logging"
	coreruntime "github.com/rafi/gits/internal/runtime"
	"github.com/rafi/gits/internal/runtime/projects"
)

// Result is what refreshing one Project produced: where its repositories
// were discovered from, where they live locally, how many came back with
// Sub-projects included, and which of those the dropped cache did not have.
type Result struct {
	Source  domain.ProviderSource
	Path    string
	Repos   int
	Flushed bool
	// New lists the repositories the refresh introduced, in Traversal order.
	// Empty when there was no earlier cache to compare with: without one,
	// every repository would count as new, which says nothing.
	New []domain.Repository
}

// Targets returns the Projects a sync run will visit, in alphabetical order:
// the ones named, or every remote-backed Project when no name is given.
//
// Only Projects discovered from a remote code forge are visited. A
// filesystem project is read from disk every time a command runs, so there
// is no stale copy of it to refresh, and one that lists its repositories by
// hand has nothing to discover at all. Naming such a project explicitly is
// the same as naming one that does not exist: there is nothing to sync.
//
// A name matching nothing is a warning rather than a failure, and it is
// reported here — before any cache is dropped — so a typo costs nothing.
func Targets(names []string, rt coreruntime.Runtime) ([]string, error) {
	targets := []string{}
	for _, name := range rt.Projects.SortedNames() {
		if len(names) > 0 && !slices.Contains(names, name) {
			continue
		}
		source := rt.Projects[name].Source
		if source == nil || !providers.IsRemote(source.Type) {
			continue
		}
		targets = append(targets, name)
	}
	if len(names) > 0 && len(targets) == 0 {
		return nil, domain.NewWarning(
			"no remote projects found matching %q", strings.Join(names, ", "))
	}
	return targets, nil
}

// One drops the Project's cache and reloads it, which repopulates that cache
// from its Provider Source.
func One(name string, rt coreruntime.Runtime) (Result, error) {
	var result Result
	var previous map[string]bool
	project := rt.Projects[name]
	if providers.HasCache(project.Source) {
		previous = cachedRepos(project, rt)
		if err := rt.Cache.Flush(project); err != nil {
			return result, fmt.Errorf("unable to remove cache: %w", err)
		}
		result.Flushed = true
	}

	loaded, err := projects.LoadOne(name, rt)
	if err != nil {
		return result, err
	}
	if loaded.Source != nil {
		result.Source = *loaded.Source
	}
	result.Path = loaded.AbsPath
	result.Repos = loaded.CountRepos()
	if previous != nil {
		result.New = newRepos(loaded, previous)
	}
	return result, nil
}

// cachedRepos reads the repositories the Project's cache holds before it is
// dropped, keyed by repoKey, or nil when there is nothing to compare with:
// no cache entry, caching turned off (a file left from before is stale by an
// unknown amount), or an entry that cannot be read. None of these fails the
// sync; they only mean no repository is reported as new.
func cachedRepos(project domain.Project, rt coreruntime.Runtime) map[string]bool {
	if rt.Settings.Cache != nil && !*rt.Settings.Cache {
		return nil
	}
	cached, found, err := rt.Cache.Peek(project.Source.UniqueKey())
	if err != nil {
		logging.Or(rt.Log).Debug("unable to read cache before sync",
			"project", project.Name, "err", err)
		return nil
	}
	if !found {
		return nil
	}
	keys := map[string]bool{}
	walkRepos(cached, project.Source.UniqueKey(),
		func(repo domain.Repository) { keys[repoKey(repo)] = true })
	return keys
}

// newRepos returns the repositories in project, Sub-projects included, whose
// key is not in previous.
func newRepos(project domain.Project, previous map[string]bool) []domain.Repository {
	var found []domain.Repository
	walkRepos(project, project.Source.UniqueKey(), func(repo domain.Repository) {
		if !previous[repoKey(repo)] {
			found = append(found, repo)
		}
	})
	return found
}

// walkRepos visits every repository in project discovered from the Provider
// Source cached under sourceKey, in Traversal order: a Project's own
// repositories before its Sub-projects, depth-first. A Sub-project declaring
// a source of its own is skipped: it is cached apart from its parent, so its
// repositories are never in the entry being compared with.
func walkRepos(project domain.Project, sourceKey string, visit func(domain.Repository)) {
	for _, repo := range project.Repos {
		visit(repo)
	}
	for _, sub := range project.SubProjects {
		if sub.Source != nil && sub.Source.UniqueKey() != sourceKey {
			continue
		}
		walkRepos(sub, sourceKey, visit)
	}
}

// repoKey identifies a repository across two fetches from the same Provider
// Source: the code forge's own ID when it gave one, so a renamed repository
// is not mistaken for a new one, and its namespaced name otherwise.
func repoKey(repo domain.Repository) string {
	if repo.ID != "" {
		return "id:" + repo.ID
	}
	return "name:" + repo.GetNameWithNamespace()
}
