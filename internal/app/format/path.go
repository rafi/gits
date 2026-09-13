// Package format renders values as text for any front end. Nothing here is
// styled, so a non-terminal client shares the same output shapes.
package format

import (
	"path/filepath"
	"strings"

	"github.com/rafi/gits/domain"
)

// Path returns a clean path with ~ for the home directory. The substitution
// is a directory match, not a string prefix: a sibling that merely shares the
// home directory's textual prefix (e.g. /Users/rafibar under /Users/rafi) is
// left untouched, and an empty homeDir never matches.
func Path(path, homeDir string) string {
	path = filepath.Clean(path)
	if homeDir == "" {
		return path
	}
	if path == homeDir {
		return "~"
	}
	if rest, cut := strings.CutPrefix(path, homeDir+string(filepath.Separator)); cut {
		return "~" + string(filepath.Separator) + rest
	}
	return path
}

// RepoRelPath returns a repository's display path: its configured Dir or its
// absolute path made relative to the project root, with ~ for the home
// directory. Every title and width computation derives from this one place.
func RepoRelPath(project domain.Project, repo domain.Repository, homeDir string) string {
	repoPath := repo.Dir
	if repoPath == "" {
		repoPath = repo.AbsPath
	}
	repoPath = strings.TrimPrefix(repoPath, project.AbsPath+"/")
	return Path(repoPath, homeDir)
}
