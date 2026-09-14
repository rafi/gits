package command

import (
	"errors"
	"fmt"

	"github.com/rafi/gits/domain"
)

// The sentinel errors a non-OK Repo State is reported as when the state
// carries no Reason of its own.
var (
	ErrNotRepository = fmt.Errorf("not a repository")
	ErrNotCloned     = fmt.Errorf("not cloned")
)

// StateError maps a non-OK repository state to its error: the Reason the
// state was classified for when it carries one, its sentinel otherwise. The
// state guard reports a turned-back repository through it.
func StateError(repo domain.Repository) error {
	switch repo.State {
	case domain.RepoStateError:
		// The error state carries the Reason it was classified for, and that
		// reason is not always "this isn't a repository" — a readable clone
		// whose git command failed lands here too. Report what actually went
		// wrong; the sentinel is the fallback when nothing said.
		if repo.Reason != "" {
			return errors.New(repo.Reason)
		}
		return ErrNotRepository
	case domain.RepoStateNotCloned:
		return ErrNotCloned
	case domain.RepoStateUnknown, domain.RepoStateRemoteOnly, domain.RepoStateOK:
		// Not error states. A caller reaching here asked for the error of a
		// repository that has none; report the state itself rather than
		// inventing one.
		fallthrough
	default:
		return errors.New(string(repo.State))
	}
}

// RepoError wraps a repo failure as a *domain.Warning (ErrorType) so it counts
// as a real error and matches uniformly via [errors.As].
func RepoError(err error, repo domain.Repository) error {
	return &domain.Warning{
		Title:  repo.GetName(),
		Reason: err.Error(),
		Dir:    repo.AbsPath,
		Cause:  err,
	}
}
