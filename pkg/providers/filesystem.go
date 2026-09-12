package providers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/karrick/godirwalk"
	"github.com/mitchellh/go-homedir"
	log "github.com/sirupsen/logrus"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/pkg/git"
)

type filesystemProvider struct {
	gitClient git.GitClient
}

func newFilesystemProvider(opts Options) *filesystemProvider {
	return &filesystemProvider{gitClient: opts.GitClient}
}

func NewFilesystemRepo(ctx context.Context, path, remote string, gitClient git.GitClient) (domain.Repository, error) {
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
func WalkRepos(ctx context.Context, root string, gitClient git.GitClient, fn func(path string) error) error {
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
		ErrorCallback: func(path string, err error) godirwalk.ErrorAction {
			_, err = fmt.Fprintf(os.Stderr, "ERROR during directory %s scan: %s\n", path, err)
			if err != nil {
				log.Errorf("WalkRepos: %s", err)
				return godirwalk.Halt
			}
			return godirwalk.SkipNode
		},
	})
}
