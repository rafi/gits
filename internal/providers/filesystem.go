package providers

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/karrick/godirwalk"
	"github.com/mitchellh/go-homedir"
	log "github.com/sirupsen/logrus"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/git"
)

type filesystemProvider struct {
	gitClient git.Client
}

func newFilesystemProvider(opts Options) *filesystemProvider {
	return &filesystemProvider{gitClient: opts.GitClient}
}

// NewFilesystemRepo describes the repository cloned at path, taking its
// Repo Src from remote or, when that is empty, from the clone's own Remote.
func NewFilesystemRepo(ctx context.Context, path, remote string, gitClient git.Client) (domain.Repository, error) {
	repo := domain.Repository{
		Name: filepath.Base(path),
		Dir:  path,
		Src:  remote,
	}

	absPath, err := homedir.Expand(path)
	if err != nil {
		return repo, fmt.Errorf("unable to expand path: %w", err)
	}
	if repo.Src == "" {
		repo.Src, err = gitClient.Remote(ctx, absPath)
		if err != nil {
			repo.State = domain.RepoStateError
			repo.Reason = err.Error()
		}
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
	return WalkRepos(ctx, path, c.gitClient, func(repoPath string) error {
		// TODO: create subprojects in nested directories
		repo, err := NewFilesystemRepo(ctx, repoPath, "", c.gitClient)
		if err != nil {
			return err
		}
		project.Repos = append(project.Repos, repo)
		return nil
	})
}

// WalkRepos walks root and calls fn with the path of every git repository
// found, without descending into repositories themselves.
func WalkRepos(ctx context.Context, root string, gitClient git.Client, fn func(path string) error) error {
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
		// report goes through the logger rather than the process stream: this
		// is Diagnostic Output, and WalkRepos is handed no destination to
		// write it to.
		ErrorCallback: func(path string, err error) godirwalk.ErrorAction {
			log.Errorf("error during directory %s scan: %s", path, err)
			return godirwalk.SkipNode
		},
	})
}
