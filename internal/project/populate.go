package project

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/mitchellh/go-homedir"
	log "github.com/sirupsen/logrus"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cli/types"
	"github.com/rafi/gits/pkg/git"
	"github.com/rafi/gits/pkg/providers"
)

func GetProjects(args []string, deps types.RuntimeDeps) (domain.ProjectListKeyed, error) {
	projs := domain.ProjectListKeyed{}

	// Support path based project directories.
	first := ""
	if len(args) > 0 {
		first, _ = homedir.Expand(args[0])
	}
	if len(first) > 0 && (first == "." || first[0:1] == "/" || first[0:2] == "./" || first[0:2] == "../") {
		project := newProjectFromDisk(first)
		deps.Projects = domain.ProjectListKeyed{project.Name: project}
		args = []string{}
	}

	// Filter projects and populate each with metadata and state.
	for name, proj := range deps.Projects {
		if len(args) > 0 && !slices.Contains(args, name) {
			continue
		}
		proj.Name = name
		if err := populateProject(&proj, deps); err != nil {
			return nil, err
		}
		projs[name] = proj
	}
	return projs, nil
}

func GetProject(name string, deps types.RuntimeDeps) (domain.Project, error) {
	list, err := GetProjects([]string{name}, deps)
	if err != nil {
		return domain.Project{}, err
	}
	return list[name], nil
}

func newProjectFromDisk(path string) domain.Project {
	return domain.Project{
		Name:   filepath.Base(path),
		Path:   path,
		Source: &domain.ProviderSource{Type: string(providers.ProviderFilesystem)},
	}
}

func populateProject(project *domain.Project, deps types.RuntimeDeps) error {
	filesystemType := string(providers.ProviderFilesystem)
	emptySource := (project.Source == nil || project.Source.Type == "")

	switch {
	case emptySource && len(project.Repos) > 0 && project.Path == "":
		// Process repos individually if project doesn't have a path.
		var err error
		for repoIdx, repo := range project.Repos {
			project.Repos[repoIdx], err = providers.NewFilesystemRepo(repo.Dir, deps.Git)
			if err != nil {
				return err
			}
		}

	case emptySource && len(project.Repos) == 0:
		// Default source type of a project _with_ path is "filesystem".
		if project.Source == nil {
			project.Source = &domain.ProviderSource{
				Search: domain.SearchQuery{},
			}
		}
		project.Source.Type = filesystemType
	}

	if project.Source != nil {
		// Default search path for "filesystem" is the project path.
		if project.Source.Search.Path == "" && project.Source.Type == filesystemType {
			project.Source.Search.Path = project.Path
		}

		// Populate repos from source.
		if project.Source.Type != "" {
			if err := getSource(project, deps); err != nil {
				return err
			}
		}
	}

	computeState(project, deps.Git)
	return nil
}

// getSource populates project repos from a provider source.
func getSource(project *domain.Project, deps types.RuntimeDeps) error {
	var (
		err         error
		hasCache    bool
		shouldCache = deps.Settings.Cache == nil || *deps.Settings.Cache
	)

	if project.Source.Type == string(providers.ProviderFilesystem) {
		shouldCache = false
	}

	// Grab source filter and concat a cache key.
	id, err := project.Source.GetFilterID()
	if id == "" {
		return fmt.Errorf("project %q error: %w", project.Name, err)
	}
	cacheKey := makeCacheKey(project.Source.Type, id)
	cacheChecksum, err := md5sum(deps.Source)
	if err != nil {
		return fmt.Errorf("failed to get cache checksum: %w", err)
	}

	if shouldCache {
		if hasCache, err = getCache(cacheKey, cacheChecksum, project); err != nil {
			return fmt.Errorf("failed to get cache: %w", err)
		}
	}
	if !hasCache {
		c, err := providers.NewGitProvider(project.Source.Type, "")
		if err != nil {
			return fmt.Errorf("failed to create provider: %w", err)
		}

		if project.Source.Type == string(providers.ProviderFilesystem) {
			log.Debugf("Searching for repos at %s…", id)
		} else {
			log.Debugf("Fetching %s repos from %s…", project.Source.Type, id)
		}
		if err := c.LoadRepos(id, deps.Git, project); err != nil {
			return fmt.Errorf("failed to load repos: %w", err)
		}
		if err != nil {
			return fmt.Errorf("failed to find all projects: %w", err)
		}
		if len(project.Repos) == 0 {
			return fmt.Errorf("no repositories found for project %q", project.Name)
		}

		if shouldCache {
			if err := saveCache(cacheKey, cacheChecksum, *project); err != nil {
				return fmt.Errorf("failed to save cache: %w", err)
			}
		}
	}
	return nil
}

// computeState evaluates project's repos state.
func computeState(project *domain.Project, git git.Git) {
	var err error
	if project.Path != "" {
		project.AbsPath, err = homedir.Expand(project.Path)
		if err != nil {
			log.Warnf("unable to expand path: %s", err)
		}
	}
	for idx := range project.SubProjects {
		sub := &project.SubProjects[idx]
		if sub.Path == "" {
			sub.Path = filepath.Join(project.Path, sub.Name)
		}
		if sub.Source == nil {
			sub.Source = project.Source
		}
		computeState(sub, git)
	}

	for repoIdx := range project.Repos {
		r := &project.Repos[repoIdx]
		r.State = domain.RepoStateUnknown

		if project.Source != nil {
			r.Type = project.Source.Type
		}
		if r.Type != string(providers.ProviderFilesystem) {
			r.State = domain.RepoStateRemote
		}

		r.State = domain.RepoStateOK

		if r.Dir == "" && project.AbsPath == "" {
			r.State = domain.RepoStateNoLocal
			continue
		}

		r.AbsPath, err = getRepoAbsPath(*project, *r)
		if err != nil {
			r.State = domain.RepoStateError
			r.Reason = err.Error()
			continue
		}
		if r.AbsPath == "" {
			r.State = domain.RepoStateRemote
			continue
		}

		if r.Src == "" {
			r.Src, err = git.Remote(r.AbsPath)
			if err != nil {
				r.State = domain.RepoStateError
				r.Reason = err.Error()
			}
		}

		if _, err := os.Stat(r.AbsPath); os.IsNotExist(err) {
			r.State = domain.RepoStateNoLocal
		} else if !git.IsRepo(r.AbsPath) {
			r.State = domain.RepoStateError
			r.Reason = "Unable to load repo"
			continue
		}

	}
}

// getRepoAbsPath returns an absolute path of a repo directory.
func getRepoAbsPath(project domain.Project, repo domain.Repository) (string, error) {
	path := filepath.Clean(project.AbsPath)
	if len(repo.Dir) == 0 {
		lastSlash := strings.LastIndex(repo.Src, "/")
		if lastSlash == -1 {
			return "", fmt.Errorf("unable to get repo path %s", repo.Src)
		}
		name := repo.Src[lastSlash+1:]
		name = strings.TrimSuffix(name, filepath.Ext(name))
		return filepath.Join(path, name), nil
	}
	expanded, err := homedir.Expand(repo.Dir)
	if err != nil {
		return "", fmt.Errorf("unable to expand path: %w", err)
	}
	if string(expanded[0]) == "/" {
		path = filepath.Clean(expanded)
	} else {
		path = filepath.Join(path, expanded)
	}
	return path, nil
}
