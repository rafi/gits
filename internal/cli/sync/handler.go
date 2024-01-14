package sync

import (
	"fmt"
	"os"
	"slices"

	"github.com/rafi/gits/internal/cli/types"
	"github.com/rafi/gits/internal/project"
)

// ExecSync cleans the cache for the given projects.
//
// Args: (optional)
//   - project names
func ExecSync(args []string, deps types.RuntimeDeps) error {
	for name, p := range deps.Projects {
		if p.Source == nil || p.Source.Search == "" {
			continue
		}
		if len(args) > 0 && !slices.Contains(args, name) {
			continue
		}
		if err := project.CleanCache(p); err != nil {
			if !os.IsNotExist(err) {
				return fmt.Errorf("unable to remove cache: %w", err)
			}
			continue
		}
		fmt.Printf("Cleaned %q project cache.\n", name)
	}

	_, err := project.GetProjects(args, deps)
	if err != nil {
		return fmt.Errorf("unable to list projects: %w", err)
	}
	return nil
}
