package git

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// modifiedRe matches the leading changed-file count of `git diff --shortstat`.
var modifiedRe = regexp.MustCompile(`^\s*(\d+)`)

func (g *Git) CurrentBranch(ctx context.Context, path string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, localTimeout)
	defer cancel()

	args := []string{"rev-parse", "--abbrev-ref", "HEAD"}
	abbrRef, err := g.Exec(ctx, path, args)
	if err != nil {
		return "", fmt.Errorf("unable to find ref: %w", err)
	}
	return cleanOutput(abbrRef), nil
}

func (g *Git) UpstreamBranch(ctx context.Context, path string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, localTimeout)
	defer cancel()

	args := []string{"rev-parse", "--abbrev-ref", "@{upstream}"}
	out, err := g.Exec(ctx, path, args)
	if err != nil {
		// In a valid repo a non-zero exit means the branch has no upstream
		// configured; git writes the reason to stderr, so report the condition
		// rather than leaking that text back as the upstream name.
		return "", ErrNoUpstream
	}
	return cleanOutput(out), nil
}

// GitModified returns the number of modified files
func (g *Git) Modified(ctx context.Context, path string) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, localTimeout)
	defer cancel()

	args := []string{"diff", "--shortstat"}
	output, err := g.Exec(ctx, path, args)
	if err != nil {
		return 0, fmt.Errorf("unable to find modified diff: %w", err)
	}
	m := modifiedRe.FindStringSubmatch(string(output))
	if m == nil {
		return 0, nil
	}
	modified, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, fmt.Errorf("unable to convert string to int: %w", err)
	}
	return modified, nil
}

// Untracked returns the number of untracked files
func (g *Git) Untracked(ctx context.Context, path string) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, localTimeout)
	defer cancel()

	args := []string{"ls-files", "--others", "--exclude-standard"}
	output, err := g.Exec(ctx, path, args)
	if err != nil {
		return 0, fmt.Errorf("unable to find untracked: %w", err)
	}
	return len(splitLines(cleanOutput(output))), nil
}

// CurrentPosition returns a short log description of HEAD
func (g *Git) CurrentPosition(ctx context.Context, path string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, localTimeout)
	defer cancel()

	args := []string{"log", "-1", "--color=always", "--format=%C(auto)%D %C(242)(%aN %ar)%Creset"}
	output, err := g.Exec(ctx, path, args)
	if err != nil {
		return "", fmt.Errorf("unable to get current rev: %w", err)
	}
	return cleanOutput(output), nil
}

// Describe generates a version description based on tags and hash
func (g *Git) Describe(ctx context.Context, path string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, localTimeout)
	defer cancel()

	args := []string{"describe", "--tags", "--always"}
	output, err := g.Exec(ctx, path, args)
	if err != nil {
		return "", fmt.Errorf("unable to describe rev: %w", err)
	}
	return cleanOutput(output), nil
}

// Diff returns a formatted string of ahead/behind counts
func (g *Git) Diff(ctx context.Context, path, branch, target string) (int, int, error) {
	ctx, cancel := context.WithTimeout(ctx, localTimeout)
	defer cancel()

	args := []string{"rev-list", "--left-right", "--end-of-options", branch + "..." + target}
	output, err := g.Exec(ctx, path, args)
	if err != nil {
		return 0, 0, fmt.Errorf("unable to compute diff: %w", err)
	}
	ahead, behind := 0, 0
	for rev := range strings.SplitSeq(cleanOutput(output), "\n") {
		if rev == "" {
			continue
		}
		switch rev[0] {
		case '<':
			ahead++
		case '>':
			behind++
		}
	}
	return ahead, behind, nil
}
