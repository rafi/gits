package add

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/app"
	"github.com/rafi/gits/internal/format"
)

// resolveTargets turns the command's target arguments into the absolute paths
// of the repositories to record, in argument order and without duplicates.
// With no targets, the current directory is the one repository.
//
// Each target is one of three things, told apart in this order:
//   - a glob pattern (`backend*`), expanded here so it works quoted as well
//     as left to the shell, keeping only the matches that are git repositories;
//   - a directory that exists, which must be a git repository, unless it came
//     among several targets: that is the shape of a shell-expanded glob, so it
//     is skipped instead;
//   - anything else is a clone URL, cloned into the current directory first.
func resolveTargets(targets []string, deps app.RuntimeCLI) ([]string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("unable to get current directory: %w", err)
	}
	if len(targets) == 0 {
		targets = []string{cwd}
	}

	// A shell-expanded glob arrives as one directory per match, so with several
	// targets a non-repository is skipped like an over-matching pattern. A
	// single named directory stays an error: the user meant that one thing.
	lenient := len(targets) > 1

	paths := make([]string, 0, len(targets))
	seen := make(map[string]bool, len(targets))
	for _, target := range targets {
		matches, err := expandTarget(cwd, target, lenient, deps)
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

// expandTarget resolves one target argument to the repositories it names. When
// lenient, a directory that is not a git repository is reported and skipped
// instead of failing the command.
func expandTarget(cwd, target string, lenient bool, deps app.RuntimeCLI) ([]string, error) {
	path := absPath(cwd, target)

	if isGlob(target) {
		return expandGlob(target, path, deps)
	}

	if info, err := os.Stat(path); err == nil {
		if !info.IsDir() || !deps.Git.IsRepo(deps.Ctx, path) {
			if lenient {
				fmt.Fprintf(deps.Err, "%s is not a git repository, skipping\n",
					format.Path(path, deps.HomeDir))
				return nil, nil
			}
			return nil, fmt.Errorf("not a git repository: %s",
				format.Path(path, deps.HomeDir))
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

// expandGlob returns the git repositories a pattern matches. Patterns are
// expected to over-match, so a match that is not a repository is skipped with
// a note rather than failing the command; a pattern matching nothing is noted
// too, and ExecAdd ends in its "nothing to add" warning if no target yielded a
// repository. A malformed pattern remains an error.
func expandGlob(pattern, absPattern string, deps app.RuntimeCLI) ([]string, error) {
	matches, err := filepath.Glob(absPattern)
	if err != nil {
		return nil, fmt.Errorf("invalid pattern %q: %w", pattern, err)
	}
	repos := make([]string, 0, len(matches))
	skipped := make([]string, 0, len(matches))
	for _, match := range matches {
		info, err := os.Stat(match)
		if err != nil || !info.IsDir() || !deps.Git.IsRepo(deps.Ctx, match) {
			skipped = append(skipped, filepath.Base(match))
			continue
		}
		repos = append(repos, filepath.Clean(match))
	}
	if len(skipped) > 0 {
		fmt.Fprintf(deps.Err, "%q matched %s, skipping: %s\n", pattern,
			plural(len(skipped), "1 path that is not a git repository",
				fmt.Sprintf("%d paths that are not git repositories", len(skipped))),
			nameList(skipped))
	}
	if len(repos) == 0 && len(matches) == 0 {
		fmt.Fprintf(deps.Err, "skipping %q, which matched nothing\n", pattern)
	}
	return repos, nil
}

// maxNamedSkips is how many skipped names are listed before the rest are
// counted, keeping the note to one short line.
const maxNamedSkips = 5

// nameList names the first few skipped paths and counts the rest.
func nameList(names []string) string {
	if len(names) <= maxNamedSkips {
		return strings.Join(names, ", ")
	}
	return fmt.Sprintf("%s and %d more",
		strings.Join(names[:maxNamedSkips], ", "), len(names)-maxNamedSkips)
}

// plural picks the form that agrees with n.
func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
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
