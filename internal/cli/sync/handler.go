package sync

import (
	"fmt"
	"slices"

	"github.com/rafi/gits/internal/cli/types"
	"github.com/rafi/gits/internal/project"
)

func ExecSync(include []string, deps types.RuntimeDeps) error {
	for _, p := range deps.Projects {
		if p.Source == nil {
			continue
		}
		if len(include) > 0 && !slices.Contains(include, p.Name) {
			continue
		}
		if err := project.CleanCache(p); err != nil {
			return fmt.Errorf("unable to remove cache: %w", err)
		}
	}

	_, err := project.GetProjects(include, deps)
	if err != nil {
		return fmt.Errorf("unable to list projects: %w", err)
	}
	return nil
}
