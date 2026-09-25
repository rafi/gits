// Package cache stores a Project's provider-discovered repositories between
// runs, so a Provider Source is not queried on every command.
package cache

import (
	"github.com/rafi/gits/domain"
)

// Cacher stores and retrieves a Project's discovered repositories under a
// key derived from its Provider Source.
type Cacher interface {
	Get(key string, project *domain.Project) (bool, error)
	// Peek reads whatever repositories are stored under key, however old,
	// without the checks that make Get treat an entry as a miss. It is for
	// comparing against, never for serving a command.
	Peek(key string) (domain.Project, bool, error)
	Save(key string, project domain.Project) error
	Flush(project domain.Project) error
}
