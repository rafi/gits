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
	coreruntime "github.com/rafi/gits/internal/runtime"
	"github.com/rafi/gits/internal/runtime/projects"
)

// Result is what refreshing one Project produced: where its repositories
// were discovered from, where they live locally, and how many came back with
// Sub-projects included.
type Result struct {
	Source  domain.ProviderSource
	Path    string
	Repos   int
	Flushed bool
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
	project := rt.Projects[name]
	if providers.HasCache(project.Source) {
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
	return result, nil
}
