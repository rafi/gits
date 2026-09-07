package add

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func expandRepositoryTargets(
	cwd string,
	targets []string,
	isRepo func(string) bool,
) ([]string, error) {
	paths := make([]string, 0, len(targets))
	seen := make(map[string]struct{})

	for _, target := range targets {
		matches := []string{resolvePath(cwd, target)}
		if hasGlobMeta(target) {
			var err error
			matches, err = filepath.Glob(resolvePath(cwd, target))
			if err != nil {
				return nil, fmt.Errorf("invalid repository pattern %q: %w", target, err)
			}
			if len(matches) == 0 {
				return nil, fmt.Errorf("repository pattern %q matched no paths", target)
			}
		}

		for _, path := range matches {
			path = filepath.Clean(path)
			info, err := os.Stat(path)
			if err != nil {
				return nil, fmt.Errorf("unable to inspect repository %q: %w", path, err)
			}
			if !info.IsDir() || !isRepo(path) {
				continue
			}
			if _, exists := seen[path]; exists {
				continue
			}
			seen[path] = struct{}{}
			paths = append(paths, path)
		}
	}

	if len(paths) == 0 {
		return nil, fmt.Errorf("no Git repositories matched")
	}
	return paths, nil
}

func resolvePath(cwd, path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Join(cwd, path)
}

func hasGlobMeta(path string) bool {
	return strings.ContainsAny(path, "*?[")
}
