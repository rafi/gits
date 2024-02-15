package sync

import (
	"fmt"
	"os"
	"slices"

	"github.com/rafi/gits/internal/loader"
	"github.com/rafi/gits/internal/types"
)

// ExecSync cleans the cache for the given projects.
//
// Args: (optional)
//   - project names
func ExecSync(args []string, deps types.RuntimeCLI) error {
	for name, p := range deps.Projects {
		if p.Source == nil || p.Source.Search == "" {
			continue
		}
		if len(args) > 0 && !slices.Contains(args, name) {
			continue
		}
		if err := deps.Cache.Flush(p); err != nil {
			if !os.IsNotExist(err) {
				return fmt.Errorf("unable to remove cache: %w", err)
			}
			continue
		}
		fmt.Printf("Cleaned %q project cache.\n", name)
	}

	_, err := loader.GetProjects(args, deps.Runtime)
	if err != nil {
		return fmt.Errorf("unable to list projects: %w", err)
	}
	return nil
}
