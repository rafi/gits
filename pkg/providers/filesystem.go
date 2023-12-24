package providers

import (
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

func newFilesystemProvider() (*filesystemProvider, error) {
	provider := &filesystemProvider{
		sourceType: ProviderFilesystem,
	}
	return provider, nil
}

func NewFilesystemRepo(path string, gitClient git.Git) (domain.Repository, error) {
	repo := domain.Repository{
		Name: filepath.Base(path),
		Dir:  path,
	}
	absPath, err := homedir.Expand(path)
	if err != nil {
		return repo, fmt.Errorf("unable to expand path: %w", err)
	}
	repo.Src, err = gitClient.Remote(absPath)
	if err != nil {
		repo.State = domain.RepoStateError
		repo.Reason = err.Error()
	}
	return repo, nil
}

func (c *filesystemProvider) LoadRepos(path string, gitClient git.Git, project *domain.Project) error {
	var err error
	path, err = homedir.Expand(path)
	if err != nil {
		return fmt.Errorf("unable to expand path: %w", err)
	}
	project.ID = path
	return godirwalk.Walk(path, &godirwalk.Options{
		Unsorted:            false,
		FollowSymbolicLinks: false,
		Callback: func(path string, de *godirwalk.Dirent) error {
			if de.IsDir() {
				_, err := os.Stat(filepath.Join(path, ".git"))
				if !os.IsNotExist(err) {
					repo, err := NewFilesystemRepo(path, gitClient)
					if err != nil {
						return err
					}
					project.Repos = append(project.Repos, repo)
					return filepath.SkipDir
				}
			}
			return nil
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
