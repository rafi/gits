package run

import (
	"errors"
	"fmt"
	"testing"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/types"
)

// TestRepoStateErrorSurfacesReason proves the Reason carried by an `error`
// repository reaches the user instead of being flattened into "not a
// repository". A repo whose git command failed — a missing git binary, say —
// is still a repository, and saying otherwise sends the reader looking in the
// wrong place.
func TestRepoStateErrorSurfacesReason(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		repo domain.Repository
		want string
	}{
		{
			"reason is surfaced verbatim",
			domain.Repository{
				State:  domain.RepoStateError,
				Reason: "unable to get remote URL: git executable not found in PATH",
			},
			"unable to get remote URL: git executable not found in PATH",
		},
		{
			"error without a reason falls back to the sentinel",
			domain.Repository{State: domain.RepoStateError},
			ErrNotRepository.Error(),
		},
		{
			"not-cloned is unchanged",
			domain.Repository{State: domain.RepoStateNotCloned, Reason: "ignored"},
			ErrNotCloned.Error(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := StateError(tt.repo).Error(); got != tt.want {
				t.Errorf("StateError() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestRepoErrorIsPointerWarning verifies RepoError yields a *types.Warning so
// it matches uniformly via [errors.As], and that it is an ErrorType — a real
// failure that counts toward the exit code, not a downgraded warning.
func TestRepoErrorIsPointerWarning(t *testing.T) {
	t.Parallel()

	repo := domain.Repository{Name: "acme", AbsPath: "/tmp/acme"}
	err := RepoError(fmt.Errorf("fetch failed"), repo)

	var warning *types.Warning
	if !errors.As(err, &warning) {
		t.Fatalf("RepoError() = %T, want it to match *types.Warning", err)
	}
	if warning.Title != repo.GetName() || warning.Dir != repo.AbsPath {
		t.Errorf("RepoError() = %+v, want it to name the repository", warning)
	}
	if types.IsWarning(err) {
		t.Error("RepoError should count as a real error, but was excluded as a warning")
	}
}
