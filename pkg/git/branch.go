package git

import (
	"context"
	"fmt"
	"slices"
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

// RemoteBranches lists remote-tracking refs as "<remote>/<branch>" names.
// Names come from the full refname with the refs/remotes/ prefix stripped,
// not %(refname:short), which would abbreviate a remote HEAD such as
// refs/remotes/origin/HEAD to just "origin".
func (g *Git) RemoteBranches(ctx context.Context, path string) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, localTimeout)
	defer cancel()

	args := []string{"for-each-ref", "--format=%(refname)", "refs/remotes"}
	output, err := g.Exec(ctx, path, args)
	if err != nil {
		return nil, fmt.Errorf("unable to list remote branches: %w", err)
	}
	refs := splitLines(cleanOutput(output))
	for i, ref := range refs {
		refs[i] = strings.TrimPrefix(ref, "refs/remotes/")
	}
	return refs, nil
}

// FallbackRef returns the remote-tracking ref conventionally comparable to
// branch when no upstream is configured: "origin/<branch>" when origin has
// it, otherwise the first configured remote with a matching remote branch.
// Empty when no remote qualifies. A detached HEAD compares against the
// remote's HEAD (its default branch) when that ref exists. One ref listing
// answers for every remote, instead of a show-ref probe per remote.
func (g *Git) FallbackRef(ctx context.Context, path, branch string) string {
	if branch == "" {
		return ""
	}
	refs, err := g.RemoteBranches(ctx, path)
	if err != nil {
		return ""
	}
	found := make(map[string]bool, len(refs))
	remotes := []string{}
	for _, ref := range refs {
		// Remote names cannot contain "/", so everything before the first
		// separator is the remote name.
		remote, _, ok := strings.Cut(ref, "/")
		if !ok {
			continue
		}
		found[ref] = true
		if !slices.Contains(remotes, remote) {
			remotes = append(remotes, remote)
		}
	}
	slices.Sort(remotes)
	slices.SortStableFunc(remotes, func(a, b string) int {
		switch {
		case a == "origin":
			return -1
		case b == "origin":
			return 1
		default:
			return 0
		}
	})
	for _, remote := range remotes {
		if found[remote+"/"+branch] {
			return remote + "/" + branch
		}
	}
	return ""
}

// Checkout switches the working tree to branch. Git's DWIM creates a tracking
// branch from a remote when the branch exists only on a remote. No local
// timeout: switching a large working tree can legitimately exceed one, and
// killing git mid-checkout risks a corrupted tree — the caller context still
// cancels it (gracefully) on Ctrl-C.
func (g *Git) Checkout(ctx context.Context, path, branch string) error {
	args := []string{"checkout", "--end-of-options", branch}
	// Exec already folds git's stderr into err, so the output is not repeated.
	if _, err := g.Exec(ctx, path, args); err != nil {
		return fmt.Errorf("unable to checkout %s: %w", branch, err)
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
