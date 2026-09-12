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
	sourceType Provider
}

func newFilesystemProvider() (*filesystemProvider, error) { // nolint:unparam
	provider := &filesystemProvider{
		sourceType: ProviderFilesystem,
	}
	return provider, nil
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

func (c *filesystemProvider) LoadRepos(ctx context.Context, path string, gitClient git.GitClient, project *domain.Project) error {
	var err error
	path, err = homedir.Expand(path)
	if err != nil {
		return err
	}
	project.ID = path
	return godirwalk.Walk(path, &godirwalk.Options{
		Unsorted:            false,
		FollowSymbolicLinks: false,
		Callback: func(path string, de *godirwalk.Dirent) error {
			if !de.IsDir() || !gitClient.IsRepo(ctx, path) {
				return nil
			}
			// TODO: create subprojects in nested directories
			repo, err := NewFilesystemRepo(ctx, path, "", gitClient)
			if err != nil {
				return err
			}
			project.Repos = append(project.Repos, repo)
			return filepath.SkipDir
		},
		ErrorCallback: func(path string, err error) godirwalk.ErrorAction {
			_, err = fmt.Fprintf(os.Stderr, "ERROR during directory %s scan: %s\n", path, err)
			if err != nil {
				log.Errorf("LoadRepos: %s", err)
				return godirwalk.Halt
			}
			return godirwalk.SkipNode
		},
	})
}
