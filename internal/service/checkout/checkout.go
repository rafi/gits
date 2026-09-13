// Package checkout lists and switches a repository's branches. It holds no
// prompt and no writer: which branch to move to is the caller's decision, so
// a TUI can drive the same two steps a terminal session does.
package checkout

import (
	"context"
	"errors"
	"fmt"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/service"
)

// Branches returns the branches available in a repository, and the one
// currently checked out.
func Branches(ctx context.Context, repo domain.Repository, rt service.Runtime) (
	[]string, string, error,
) {
	current, err := rt.Git.CurrentBranch(ctx, repo.AbsPath)
	if err != nil {
		return nil, "", fmt.Errorf("unable to get branch: %w", err)
	}

	branches, err := rt.Git.AllBranches(ctx, repo.AbsPath)
	if err != nil {
		return nil, "", fmt.Errorf("unable to read branches: %w", err)
	}
	// An option-less select cannot be submitted, only aborted. Defensive: a
	// repository with no commits already failed above, in CurrentBranch.
	if len(branches) == 0 {
		return nil, "", errors.New("repository has no branches")
	}
	return branches, current, nil
}

// Switch checks a branch out. A no-op when the branch is already the one
// checked out, so a caller that did not compare cannot produce a spurious
// checkout.
func Switch(ctx context.Context, repo domain.Repository, want string, rt service.Runtime) error {
	current, err := rt.Git.CurrentBranch(ctx, repo.AbsPath)
	if err != nil {
		return fmt.Errorf("unable to get branch: %w", err)
	}
	if current == want {
		return nil
	}
	return rt.Git.Checkout(ctx, repo.AbsPath, want)
}
