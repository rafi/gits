// Package orphan finds git repositories on disk that a Project's config does
// not declare. It writes nothing and renders nothing: the repositories it
// returns are data, so a TUI lists the same ones a terminal session does.
package orphan

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/karrick/godirwalk"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/infra/git"
	"github.com/rafi/gits/internal/infra/providers"
	"github.com/rafi/gits/internal/logging"
	coreruntime "github.com/rafi/gits/internal/runtime"
)

// InProject scans a Project's path for repositories its config does not
// declare. A Project with no path cannot be scanned: every repository under
// it has an absolute path of its own, so there is no directory to walk.
func InProject(project domain.Project, rt coreruntime.Runtime) ([]domain.Repository, error) {
	if project.AbsPath == "" {
		return nil, fmt.Errorf(
			"project %q has no path, so every repository has an absolute path, aborting",
			project.Name,
		)
	}

	known := make(map[string]bool)
	declaredPaths(project, known)

	found := []domain.Repository{}
	err := providers.WalkRepos(rt.Ctx, rt.Log, project.AbsPath, rt.Git, func(path string) error {
		if known[path] {
			return nil
		}
		repo, err := newRepo(rt.Ctx, path, rt.Git)
		if err != nil {
			return err
		}
		found = append(found, repo)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return found, nil
}

// InWorkTree scans one repository's work tree for git repositories nested
// inside it. Detection is by `.git` presence rather than by asking git:
// inside a work tree every subdirectory reports --is-inside-work-tree.
func InWorkTree(root string, rt coreruntime.Runtime) ([]domain.Repository, error) {
	logger := logging.Or(rt.Log)
	found := []domain.Repository{}
	err := godirwalk.Walk(root, &godirwalk.Options{
		Unsorted:            false,
		FollowSymbolicLinks: false,
		Callback: func(path string, de *godirwalk.Dirent) error {
			if !de.IsDir() || path == root {
				return nil
			}
			if de.Name() == ".git" {
				return filepath.SkipDir
			}
			// A stat error simply means no .git here: keep descending.
			if !isRepoDir(path) {
				return nil
			}
			repo, err := newRepo(rt.Ctx, path, rt.Git)
			if err != nil {
				return err
			}
			found = append(found, repo)
			return filepath.SkipDir
		},
		ErrorCallback: func(path string, err error) godirwalk.ErrorAction {
			logger.WarnContext(rt.Ctx, "skipping unreadable directory",
				"path", path, "err", err)
			return godirwalk.SkipNode
		},
	})
	if err != nil {
		return nil, err
	}
	return found, nil
}

// isRepoDir reports whether a directory holds its own .git.
func isRepoDir(path string) bool {
	_, err := os.Stat(filepath.Join(path, ".git"))
	return err == nil
}

// newRepo describes a discovered directory as a Repository, filling its Repo
// Src from the clone's remote. A failed lookup leaves Src empty; the
// directory is what `orphan` is reporting and stands on its own.
func newRepo(ctx context.Context, path string, gitClient git.Reader) (domain.Repository, error) {
	repo, err := providers.NewFilesystemRepo(path, "")
	if err != nil {
		return domain.Repository{}, err
	}
	if repo.Src == "" {
		if src, err := gitClient.Remote(ctx, repo.Dir); err == nil {
			repo.Src = src
		}
	}
	return repo, nil
}

// declaredPaths collects every repository path the Project declares, its
// sub-projects included. The walk it feeds compares filesystem paths, so this
// is keyed by AbsPath rather than by domain.Repository.Key.
func declaredPaths(project domain.Project, out map[string]bool) {
	for _, repo := range project.Repos {
		out[repo.AbsPath] = true
	}
	for _, sub := range project.SubProjects {
		declaredPaths(sub, out)
	}
}
