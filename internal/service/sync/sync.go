// Package sync refreshes each Project's cached repository list from its
// Provider Source. It writes nothing: which projects were flushed is
// returned, so the caller decides whether and how to say so.
package sync

import (
	"fmt"
	"slices"
	"strings"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/infra/providers"
	"github.com/rafi/gits/internal/service"
	"github.com/rafi/gits/internal/service/catalog"
)

// Sync drops the cache for the named projects — every cacheable one when no
// name is given — and reloads them, which repopulates the cache from each
// Provider Source. It returns the projects whose cache it dropped, in the
// order it dropped them.
func Sync(names []string, rt service.Runtime) ([]string, error) {
	flushed := []string{}
	for name, project := range rt.Projects {
		if !providers.HasCache(project.Source) {
			continue
		}
		if len(names) > 0 && !slices.Contains(names, name) {
			continue
		}
		if err := rt.Cache.Flush(project); err != nil {
			return flushed, fmt.Errorf("unable to remove cache: %w", err)
		}
		flushed = append(flushed, name)
	}

	projects, err := catalog.Load(names, rt)
	if err != nil {
		return flushed, fmt.Errorf("unable to list projects: %w", err)
	}
	// A name that matched nothing is a warning rather than a failure: the
	// caches that did match were still dropped and reloaded.
	if len(names) > 0 && len(projects) == 0 {
		return flushed, domain.NewWarning("no projects found matching %q", strings.Join(names, ", "))
	}
	return flushed, nil
}
