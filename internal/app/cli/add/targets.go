package add

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/app"
)

// resolveTargets turns the command's target arguments into the absolute paths
// of the repositories to record, in argument order and without duplicates.
// With no targets, the current directory is the one repository.
//
// Each target is one of three things, told apart in this order:
//   - a glob pattern (`backend*`), expanded here so it works quoted as well
//     as left to the shell, keeping only the matches that are git repositories;
//   - a directory that exists, which must be a git repository;
//   - anything else is a clone URL, cloned into the current directory first.
func resolveTargets(targets []string, deps app.RuntimeCLI) ([]string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("unable to get current directory: %w", err)
	}
	if len(targets) == 0 {
		targets = []string{cwd}
	}

	paths := make([]string, 0, len(targets))
	seen := make(map[string]bool, len(targets))
	for _, target := range targets {
		matches, err := expandTarget(cwd, target, deps)
		if err != nil {
			return nil, err
		}
		for _, path := range matches {
			if seen[path] {
				continue
			}
			seen[path] = true
			paths = append(paths, path)
		}
	}
	return paths, nil
}

// expandTarget resolves one target argument to the repositories it names.
func expandTarget(cwd, target string, deps app.RuntimeCLI) ([]string, error) {
	path := absPath(cwd, target)

	if isGlob(target) {
		return expandGlob(target, path, deps)
	}

	if info, err := os.Stat(path); err == nil {
		if !info.IsDir() || !deps.Git.IsRepo(deps.Ctx, path) {
			return nil, fmt.Errorf("not a git repository: %s", path)
		}
		return []string{path}, nil
	}

	if !looksLikeCloneURL(target) {
		return nil, fmt.Errorf(
			"%q is neither a directory, a glob pattern, nor a clone URL", target)
	}

	// Clone into the current directory, under the name derived from the URL.
	path = filepath.Join(cwd, domain.RepoDirName(target))
	output, err := deps.Git.Clone(deps.Ctx, target, path)
	if err != nil {
		// git's own account of the failure, which explains the error returned
		// rather than being anything the command was asked for.
		fmt.Fprintln(deps.Err, output)
		return nil, err
	}
	return []string{path}, nil
}

// expandGlob returns the git repositories a pattern matches. Matches that are
// not repositories — a plain directory, a file — are passed over silently: a
// pattern is expected to over-match, and naming each miss would bury the
// result. Matching nothing at all is a failure.
func expandGlob(pattern, absPattern string, deps app.RuntimeCLI) ([]string, error) {
	matches, err := filepath.Glob(absPattern)
	if err != nil {
		return nil, fmt.Errorf("invalid pattern %q: %w", pattern, err)
	}
	repos := make([]string, 0, len(matches))
	for _, match := range matches {
		info, err := os.Stat(match)
		if err != nil || !info.IsDir() || !deps.Git.IsRepo(deps.Ctx, match) {
			continue
		}
		repos = append(repos, filepath.Clean(match))
	}
	if len(repos) == 0 {
		return nil, fmt.Errorf("pattern %q matched no git repositories", pattern)
	}
	return repos, nil
}

// absPath resolves a target against the working directory.
func absPath(cwd, target string) string {
	if filepath.IsAbs(target) {
		return filepath.Clean(target)
	}
	return filepath.Join(cwd, target)
}

// isGlob reports whether a target carries any of the characters
// [filepath.Match] treats as a pattern.
func isGlob(target string) bool {
	return strings.ContainsAny(target, "*?[")
}

// looksLikeCloneURL reports whether a target that names nothing on disk is
// plausibly a clone URL: `scheme://…` or scp-style `host:path`. A bare word
// is more likely a mistyped directory than a URL, and is refused rather than
// handed to git to fail on.
func looksLikeCloneURL(target string) bool {
	return strings.Contains(target, ":")
}
