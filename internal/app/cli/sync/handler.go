// Package sync implements `gits sync`, which refreshes each Project's cached
// repository list from its Provider Source.
package sync

import (
	"fmt"

	"github.com/rafi/gits/internal/app"
	"github.com/rafi/gits/internal/service/sync"
)

// ExecSync refreshes the cache for the given projects.
//
// Args: (optional)
//   - project names
func ExecSync(args []string, deps app.RuntimeCLI) error {
	flushed, err := sync.Sync(args, deps.Runtime)
	// The lines are printed before the error is returned: a failure partway
	// through still dropped the caches named above it.
	for _, name := range flushed {
		fmt.Fprintf(deps.Out, "Cleaned %q project cache.\n", name)
	}
	return err
}
