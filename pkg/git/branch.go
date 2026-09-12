package git

import (
	"context"
	"fmt"
	"strings"
)

// Branches lists local branch short-names.
func (g *Git) Branches(ctx context.Context, path string) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, localTimeout)
	defer cancel()

	args := []string{"for-each-ref", "--format=%(refname:short)", "refs/heads"}
	output, err := g.Exec(ctx, path, args)
	if err != nil {
		return nil, fmt.Errorf("unable to list branches: %w", err)
	}
	return splitLines(cleanOutput(output)), nil
}

// AllBranches lists checkout-able branch names: local branches first, then
// branches existing only on a remote, with the remote prefix stripped so
// selecting one lets git's DWIM create a local tracking branch. Symbolic
// entries like origin/HEAD are dropped, and a branch present both locally
// and remotely appears once.
func (g *Git) AllBranches(ctx context.Context, path string) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, localTimeout)
	defer cancel()

	args := []string{"for-each-ref", "--format=%(refname)", "refs/heads", "refs/remotes"}
	output, err := g.Exec(ctx, path, args)
	if err != nil {
		return nil, fmt.Errorf("unable to list branches: %w", err)
	}

	// for-each-ref sorts by refname, so refs/heads precede refs/remotes.
	branches := []string{}
	seen := map[string]bool{}
	for _, ref := range splitLines(cleanOutput(output)) {
		name, local := strings.CutPrefix(ref, "refs/heads/")
		if !local {
			rest, remote := strings.CutPrefix(ref, "refs/remotes/")
			if !remote {
				continue
			}
			// Remote names cannot contain "/", so everything after the first
			// separator is the branch name.
			_, name, _ = strings.Cut(rest, "/")
			if name == "" || name == "HEAD" {
				continue
			}
		}
		if !seen[name] {
			seen[name] = true
			branches = append(branches, name)
		}
	}
	return branches, nil
}

// Remotes lists configured remote names.
func (g *Git) Remotes(ctx context.Context, path string) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, localTimeout)
	defer cancel()

	args := []string{"remote"}
	output, err := g.Exec(ctx, path, args)
	if err != nil {
		return nil, fmt.Errorf("unable to list remotes: %w", err)
	}
	return splitLines(cleanOutput(output)), nil
}

// HasRemoteBranch reports whether a remote-tracking branch ref exists.
func (g *Git) HasRemoteBranch(ctx context.Context, path, remote, branch string) bool {
	ctx, cancel := context.WithTimeout(ctx, localTimeout)
	defer cancel()

	ref := fmt.Sprintf("refs/remotes/%s/%s", remote, branch)
	args := []string{"show-ref", "--verify", "--quiet", ref}
	_, err := g.Exec(ctx, path, args)
	return err == nil
}

// Checkout switches the working tree to branch. Git's DWIM creates a tracking
// branch from a remote when the branch exists only on a remote. No local
// timeout: switching a large working tree can legitimately exceed one, and
// killing git mid-checkout risks a corrupted tree — the caller context still
// cancels it (gracefully) on Ctrl-C.
func (g *Git) Checkout(ctx context.Context, path, branch string) error {
	args := []string{"checkout", "--end-of-options", branch}
	output, err := g.Exec(ctx, path, args)
	if err != nil {
		return fmt.Errorf("unable to checkout %s: %s %w", branch, output, err)
	}
	return nil
}

// splitLines splits trimmed git output into non-empty lines.
func splitLines(s string) []string {
	if s == "" {
		return []string{}
	}
	return strings.Split(s, "\n")
}
