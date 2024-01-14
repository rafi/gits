package project

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"

	"github.com/mitchellh/go-homedir"
	log "github.com/sirupsen/logrus"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cli/types"
	"github.com/rafi/gits/pkg/git"
	"github.com/rafi/gits/pkg/providers"
)

// GetProjects returns a list of populated projects filtered by name or path.
func GetProjects(args []string, deps types.RuntimeDeps) (domain.ProjectListKeyed, error) {
	projs := domain.ProjectListKeyed{}

	// Support path based project directories.
	first := ""
	if len(args) > 0 {
		first, _ = homedir.Expand(args[0])
	}
	if len(first) > 0 && (first == "." || first[0:1] == "/" || first[0:2] == "./" || first[0:2] == "../") {
		project := newFilesystemProject(first)
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

// GetProject returns a project by name or path.
func GetProject(name string, deps types.RuntimeDeps) (domain.Project, error) {
	list, err := GetProjects([]string{name}, deps)
	if err != nil {
		return domain.Project{}, err
	}
	return list[name], nil
}

func newFilesystemProject(path string) domain.Project {
	return domain.Project{
		Name: filepath.Base(path),
		Path: path,
		Source: &domain.ProviderSource{
			Type: string(providers.ProviderFilesystem),
		},
	}
}

// populateProject populates a project with repositories, metadata and state.
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
			project.Source = &domain.ProviderSource{}
		}
		project.Source.Type = filesystemType
	}

	if project.Source != nil {
		// Default search path for "filesystem" is the project path.
		if project.Source.Search == "" && project.Source.Type == filesystemType {
			project.Source.Search = project.Path
		}

		// Populate repos from source.
		if project.Source.Type != "" {
			if err := getSource(project, deps); err != nil {
				return err
			}
		}
	}

	// Load any remote sources, and check repositories state.
	computeState(project, deps.Git)

	// Filter by user include/exclude config values.
	if err := project.Filter(); err != nil {
		return err
	}
	return nil
}

// getSource populates project repos from a provider source.
func getSource(project *domain.Project, deps types.RuntimeDeps) error {
	var (
		err         error
		hasCache    bool
		shouldCache = deps.Settings.Cache == nil || *deps.Settings.Cache
		source      = project.Source
	)

	if source.Type == string(providers.ProviderFilesystem) {
		shouldCache = false
	}

	// Grab source filter and concat a cache key.
	if err := source.Validate(); err != nil {
		return fmt.Errorf("incorrect config for project %q: %w", project.Name, err)
	}
	cacheKey := project.Source.UniqueKey()
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
		c, err := providers.NewGitProvider(source.Type, "")
		if err != nil {
			return fmt.Errorf("failed to create provider: %w", err)
		}

		if source.Type == string(providers.ProviderFilesystem) {
			log.Debugf("Searching for repos at %s…", source.Search)
		} else {
			log.Debugf("Fetching %s repos from %s…", source.Type, source.Search)
		}
		if err := c.LoadRepos(source.Search, deps.Git, project); err != nil {
			return fmt.Errorf(
				"Failed to load repos for %q project (%s): %w"+
					project.Name,
				source.Type,
				err,
			)
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

		r.AbsPath, err = project.GetRepoAbsPath(*r)
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

	// Sort sub-projects and repositories alphabetically.
	sort.SliceStable(project.SubProjects, func(i, j int) bool {
		return project.SubProjects[i].Name < project.SubProjects[j].Name
	})
	sort.SliceStable(project.Repos, func(i, j int) bool {
		return project.Repos[i].Name < project.Repos[j].Name
	})
}
