// Package catalog turns configured Projects into populated ones: it discovers
// repositories from a Provider Source, expands paths, and classifies each
// Repository's Repo State.
package catalog

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/mitchellh/go-homedir"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/logging"
	"github.com/rafi/gits/internal/providers"
	"github.com/rafi/gits/internal/service"
)

// Option tunes how Load and LoadOne populate a project. Callers that
// pass none get the default behavior: a cache miss on a remote source falls
// through to the provider.
type Option func(*options)

// options is the resolved set of load tunables.
type options struct {
	// cacheOnly stops a remote Provider Source from being contacted: a cache
	// miss leaves the project without its provider repositories rather than
	// running a network fetch or a tokenCommand passphrase prompt. Shell
	// completion loads this way so pressing Tab never blocks or prompts.
	cacheOnly bool
}

// CacheOnly makes a load never contact a remote Provider Source: cached
// repositories are returned, and a miss yields an empty repository list rather
// than a provider fetch. Local filesystem discovery is offline and still runs.
func CacheOnly() Option {
	return func(o *options) { o.cacheOnly = true }
}

// Load returns a list of populated projects filtered by name or path,
// keyed by project name. A path argument names a project that is not in the
// configuration, so its key is the name derived from the path rather than the
// path the caller passed; callers needing the resolved name read it from the
// key or from Project.Name. The args slice is never modified.
func Load(args []string, deps service.Runtime, opts ...Option) (domain.ProjectListKeyed, error) {
	var o options
	for _, opt := range opts {
		opt(&o)
	}

	// names filters the configured projects. A path argument replaces the
	// candidates entirely, so it filters nothing.
	names := args

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
		names = nil
	}

	// Filter projects and populate each with metadata and state.
	projs := domain.ProjectListKeyed{}
	for name, proj := range deps.Projects {
		if len(names) > 0 && !slices.Contains(names, name) {
			continue
		}
		proj.Name = name
		if err := populateProject(&proj, deps, o); err != nil {
			return nil, err
		}
		projs[name] = proj
	}
	return projs, nil
}

// LoadOne returns a project by name or path.
func LoadOne(name string, deps service.Runtime, opts ...Option) (domain.Project, error) {
	list, err := Load([]string{name}, deps, opts...)
	if err != nil {
		return domain.Project{}, err
	}
	// Path based names won't match with the argument, so don't use list[name].
	for _, proj := range list {
		return proj, nil
	}
	return domain.Project{}, fmt.Errorf("%q not found", name)
}

// ProjectName returns the project name a Load argument resolves to: the
// argument itself for a configured project, and the name derived from the path
// for a path argument — the same name Load keys the result by. A path that
// cannot be resolved is returned unchanged, since Load will report that
// failure itself.
func ProjectName(arg string) string {
	if !isPath(arg) {
		return arg
	}
	path, err := homedir.Expand(arg)
	if err != nil {
		return arg
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return arg
	}
	return newFilesystemProject(path).Name
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

// newFilesystemProject creates a project from an absolute path.
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
func populateProject(project *domain.Project, deps service.Runtime, o options) error {
	filesystemType := string(providers.ProviderFilesystem)
	emptySource := (project.Source == nil || project.Source.Type == "")

	switch {
	case emptySource && len(project.Repos) > 0 && project.Path == "":
		// Repositories that state their own location need no discovery, and
		// keep the identity the config gave them. Pinned by
		// TestPathlessProjectKeepsRepoIdentity.

	case emptySource && len(project.Repos) == 0 && project.Path != "":
		// Default source type of a project _with_ path is "filesystem".
		// Without a path there is nothing to search: such a project either
		// only groups Sub-projects, each of which states its own location,
		// or is empty. Defaulting it to filesystem discovery anyway failed
		// validation, telling the user to set `search:` on a source they
		// never declared.
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
			if err := getSource(project, deps, o); err != nil {
				return err
			}
		}
	}

	// A Sub-project that declares a Provider Source of its own is discovered
	// from it, the same way the root project is. Only a declared source is
	// loaded: the source a sub-project *inherits* is copied later, by
	// expandPaths, and discovering through it would walk the parent's search
	// a second time and duplicate every repository the parent already found.
	if err := loadSubProjectSources(project, deps, o); err != nil {
		return err
	}

	// Resolve paths, classify each repository, then order the tree. Each is
	// its own pass: classification reads the paths expansion produces, and
	// ordering depends on neither.
	logger := logging.Or(deps.Log)
	expandPaths(deps.Ctx, logger, project)
	classifyRepos(deps.Ctx, project, deps.Git)
	sortTree(project)

	// Filter by user include/exclude config values.
	project.Filter()
	return nil
}
