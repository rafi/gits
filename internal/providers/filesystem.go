package providers

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/karrick/godirwalk"
	"github.com/mitchellh/go-homedir"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/git"
	"github.com/rafi/gits/internal/logging"
)

type filesystemProvider struct {
	gitClient git.Reader
	log       *slog.Logger
}

func newFilesystemProvider(opts Options) *filesystemProvider {
	return &filesystemProvider{gitClient: opts.GitClient, log: opts.Log}
}

// NewFilesystemRepo describes the repository cloned at path, with the Repo Src
// it was handed. When that is empty the source is left unresolved: reading it
// from the clone's own remote costs a git subprocess, which only the commands
// that display Repo Src should pay (see loader.ResolveSrc). Callers that show
// the source immediately — `orphan` — resolve it themselves.
func NewFilesystemRepo(path, remote string) (domain.Repository, error) {
	repo := domain.Repository{
		Name: filepath.Base(path),
		Dir:  path,
		Src:  remote,
	}

	if _, err := homedir.Expand(path); err != nil {
		return repo, fmt.Errorf("unable to expand path: %w", err)
	}
	return repo, nil
}

func (c *filesystemProvider) LoadRepos(ctx context.Context, path string, project *domain.Project) error {
	var err error
	path, err = homedir.Expand(path)
	if err != nil {
		return err
	}
	project.ID = path
	return WalkRepos(ctx, c.log, path, c.gitClient, func(repoPath string) error {
		// TODO: create subprojects in nested directories
		repo, err := NewFilesystemRepo(repoPath, "")
		if err != nil {
			return err
		}
		project.Repos = append(project.Repos, repo)
		return nil
	})
}

// WalkRepos walks root and calls fn with the path of every git repository
// found, without descending into repositories themselves.
func WalkRepos(
	ctx context.Context,
	logger *slog.Logger,
	root string,
	gitClient git.Reader,
	fn func(path string) error,
) error {
	logger = logging.Or(logger)
	return godirwalk.Walk(root, &godirwalk.Options{
		Unsorted:            false,
		FollowSymbolicLinks: false,
		Callback: func(path string, de *godirwalk.Dirent) error {
			if !de.IsDir() || !gitClient.IsRepo(ctx, path) {
				return nil
			}
			if err := fn(path); err != nil {
				return err
			}
			return filepath.SkipDir
		},
		// A directory that cannot be read is skipped, not fatal, and the
		// report goes to the tracer the caller passed in rather than to a
		// process stream: WalkRepos is handed no output destination.
		ErrorCallback: func(path string, err error) godirwalk.ErrorAction {
			logger.WarnContext(ctx, "skipping unreadable directory",
				"path", path, "err", err)
			return godirwalk.SkipNode
		},
	})
}
