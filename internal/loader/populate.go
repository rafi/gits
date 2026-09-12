// Package loader turns configured Projects into populated ones: it discovers
// repositories from a Provider Source, expands paths, and classifies each
// Repository's Repo State.
package loader

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/mitchellh/go-homedir"
	log "github.com/sirupsen/logrus"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/git"
	"github.com/rafi/gits/internal/providers"
	"github.com/rafi/gits/internal/types"
)

// GetProjects returns a list of populated projects filtered by name or path.
func GetProjects(args []string, deps types.Runtime) (domain.ProjectListKeyed, error) {
	// Support path based project directories. Resolve to an absolute path
	// first: the filesystem walk returns dirs relative to its root, so a
	// relative root would be joined onto itself ("dir/dir"), and "." or "~"
	// would become the project name.
	if len(args) > 0 && isPath(args[0]) {
		path, err := homedir.Expand(args[0])
		if err != nil {
			return nil, fmt.Errorf("unable to expand path: %w", err)
		}
		path, err = filepath.Abs(path)
		if err != nil {
			return nil, fmt.Errorf("unable to resolve path %q: %w", args[0], err)
		}
		project := newFilesystemProject(path)
		deps.Projects = domain.ProjectListKeyed{project.Name: project}
		args[0] = project.Name
	}

	// Filter projects and populate each with metadata and state.
	projs := domain.ProjectListKeyed{}
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
func GetProject(name string, deps types.Runtime) (domain.Project, error) {
	list, err := GetProjects([]string{name}, deps)
	if err != nil {
		return domain.Project{}, err
	}
	// Path based names won't match with the argument, so don't use list[name].
	for _, proj := range list {
		return proj, nil
	}
	return domain.Project{}, fmt.Errorf("%q not found", name)
}

// isPath checks if a string is a path.
func isPath(path string) bool {
	path, _ = homedir.Expand(path)
	switch {
	case path == "":
		return false
	case path == ".", path[0] == '/':
		return true
	case strings.HasPrefix(path, "./"), strings.HasPrefix(path, "../"):
		return true
	}
	return false
}

// newFilesystemProject creates a project from an abslute path.
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
func populateProject(project *domain.Project, deps types.Runtime) error {
	filesystemType := string(providers.ProviderFilesystem)
	emptySource := (project.Source == nil || project.Source.Type == "")

	switch {
	case emptySource && len(project.Repos) > 0 && project.Path == "":
		// Process repos individually if project doesn't have a path.
		var err error
		for repoIdx, repo := range project.Repos {
			project.Repos[repoIdx], err = providers.NewFilesystemRepo(
				deps.Ctx,
				repo.Dir,
				repo.Src,
				deps.Git,
			)
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
	computeState(deps.Ctx, project, deps.Git)

	// Filter by user include/exclude config values.
	project.Filter()
	return nil
}

// getSource populates project repos from a provider source.
func getSource(project *domain.Project, deps types.Runtime) error {
	var (
		err         error
		hasCache    bool
		source      = project.Source
		shouldCache = (deps.Settings.Cache == nil || *deps.Settings.Cache) &&
			providers.HasCache(source)
	)

	// Grab source filter and concat a cache key.
	if err := source.Validate(); err != nil {
		return fmt.Errorf("incorrect config for project %q: %w", project.Name, err)
	}
	cacheKey := project.Source.UniqueKey()
	if shouldCache {
		if err := project.CalculateHash(); err != nil {
			return err
		}
		hasCache, err = deps.Cache.Get(cacheKey, project)
		if err != nil {
			return fmt.Errorf("failed to get cache: %w", err)
		}
	}
	if hasCache {
		return nil
	}
	if err := loadFromProvider(project, deps); err != nil {
		return err
	}
	if !shouldCache {
		return nil
	}
	if err := deps.Cache.Save(cacheKey, *project); err != nil {
		return fmt.Errorf("failed to save cache: %w", err)
	}
	return nil
}

// loadFromProvider asks the project's Provider Source for its repositories,
// the path taken whenever the cache did not answer.
func loadFromProvider(project *domain.Project, deps types.Runtime) error {
	source := project.Source
	auth := deps.Settings.ProviderAuth(source.Type)
	c, err := providers.NewGitProvider(deps.Ctx, source.Type, providers.Options{
		Token:           auth.Token,
		TokenCommand:    auth.Command(),
		IncludeArchived: deps.Settings.IncludeArchived,
		Timeout:         deps.Settings.ProviderTimeoutDuration(),
		GitClient:       deps.Git,
	})
	if err != nil {
		return fmt.Errorf("failed to create provider: %w", err)
	}

	if providers.IsRemote(source.Type) {
		log.Debugf("Fetching %s repos from %s…", source.Type, source.Search)
	} else {
		log.Debugf("Searching for repos at %s…", source.Search)
	}
	if err := c.LoadRepos(deps.Ctx, source.Search, project); err != nil {
		return fmt.Errorf(
			"failed to load repos for %q project (%s): %w",
			project.Name,
			source.Type,
			err,
		)
	}
	if len(project.Repos) == 0 {
		return fmt.Errorf("no repositories found for project %q", project.Name)
	}
	return nil
}

// computeState resolves every project path, classifies each repository, and
// orders the tree. Each concern is its own pass: classification reads the
// paths expansion produces, and ordering depends on neither.
func computeState(ctx context.Context, project *domain.Project, git git.Client) {
	expandPaths(project)
	classifyRepos(ctx, project, git)
	sortTree(project)
}

// expandPaths resolves each project's configured path to an absolute one, and
// gives every sub-project the path and source it inherits from its parent.
func expandPaths(project *domain.Project) {
	if project.Path != "" {
		var err error
		project.AbsPath, err = homedir.Expand(project.Path)
		if err != nil {
			log.Warnf("unable to expand path: %s", err)
		}
	}

	for idx := range project.SubProjects {
		sub := &project.SubProjects[idx]
		// A sub-project without its own path sits beneath its parent, in a
		// directory named after it — but only when the parent has a path to
		// sit beneath. Deriving one regardless would yield the bare name,
		// which resolves against the process working directory; leaving it
		// empty lets classification see that there is no local home at all.
		if sub.Path == "" && project.Path != "" {
			sub.Path = filepath.Join(project.Path, sub.Name)
		}
		if sub.Source == nil && project.Source != nil {
			// Copy the parent source so later mutations (e.g. getSource
			// adjusting Search) never leak across the project tree.
			src := *project.Source
			sub.Source = &src
		}
		expandPaths(sub)
	}
}

// classifyRepos determines the state of every repository in the tree.
func classifyRepos(ctx context.Context, project *domain.Project, git git.Client) {
	for idx := range project.SubProjects {
		classifyRepos(ctx, &project.SubProjects[idx], git)
	}
	for idx := range project.Repos {
		classifyRepo(ctx, project, &project.Repos[idx], git)
	}
}

// classifyRepo determines one repository's state, and the reason behind it
// when that state carries one. An unusable repository is reported through its
// state rather than by failing the project it belongs to.
func classifyRepo(
	ctx context.Context,
	project *domain.Project,
	r *domain.Repository,
	git git.Client,
) {
	r.State = domain.RepoStateUnknown
	if project.Source != nil {
		r.Type = project.Source.Type
	}

	if r.Dir == "" && project.AbsPath == "" {
		// No local destination can be derived. A provider-backed repo
		// legitimately has none; anything else is a configuration that
		// never said where the repository lives.
		if providers.IsRemote(r.Type) {
			r.State = domain.RepoStateRemoteOnly
			return
		}
		r.State = domain.RepoStateError
		r.Reason = "no `path:` on the project and no `dir:` on the repository"
		return
	}

	var err error
	r.AbsPath, err = project.GetRepoAbsPath(*r)
	if err != nil {
		r.State = domain.RepoStateError
		r.Reason = err.Error()
		return
	}

	// Check existence before running any git command, so a missing clone
	// never carries a leftover error reason.
	if _, err := os.Stat(r.AbsPath); os.IsNotExist(err) {
		r.State = domain.RepoStateNotCloned
		return
	}
	if !git.IsRepo(ctx, r.AbsPath) {
		r.State = domain.RepoStateError
		r.Reason = "Unable to load repo"
		return
	}

	if r.Src == "" {
		r.Src, err = git.Remote(ctx, r.AbsPath)
		if err != nil {
			r.State = domain.RepoStateError
			r.Reason = err.Error()
			return
		}
	}

	r.State = domain.RepoStateOK
}

// sortTree orders sub-projects and repositories alphabetically at every level.
func sortTree(project *domain.Project) {
	for idx := range project.SubProjects {
		sortTree(&project.SubProjects[idx])
	}
	sort.SliceStable(project.SubProjects, func(i, j int) bool {
		return project.SubProjects[i].Name < project.SubProjects[j].Name
	})
	sort.SliceStable(project.Repos, func(i, j int) bool {
		return project.Repos[i].Name < project.Repos[j].Name
	})
}
