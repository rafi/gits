package projects

import (
	"fmt"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/infra/providers"
	"github.com/rafi/gits/internal/logging"
	coreruntime "github.com/rafi/gits/internal/runtime"
)

// loadSubProjectSources discovers every Sub-project that declares its own
// Provider Source, depth-first, so a source at any nesting level is asked for
// its repositories. A sub-project with no source of its own is left alone,
// along with everything beneath it.
func loadSubProjectSources(project *domain.Project, deps coreruntime.Runtime, o options) error {
	for idx := range project.SubProjects {
		sub := &project.SubProjects[idx]
		// Only a source the Sub-project declares itself: the one it inherits
		// is copied later, by expandPaths, and discovering through it would
		// walk the parent's search a second time. So the defaults are applied
		// to what is already there rather than derived from the path, which
		// would turn every pathed Sub-project into a filesystem search.
		if sub.Source == nil || sub.Source.Type == "" {
			continue
		}
		applySourceDefaults(sub)
		inheritAuth(project.Source, sub.Source)
		if err := getSource(sub, deps, o); err != nil {
			return err
		}
		if err := loadSubProjectSources(sub, deps, o); err != nil {
			return err
		}
	}
	return nil
}

// inheritAuth copies parent's credentials to sub when sub has none and both
// use the same provider type.
func inheritAuth(parent, sub *domain.ProviderSource) {
	if parent == nil || sub == nil {
		return
	}
	if parent.Type != sub.Type || !sub.Auth().IsZero() || parent.Auth().IsZero() {
		return
	}
	sub.Token = parent.Token
	sub.TokenCommand = parent.TokenCommand
	sub.TokenCmd = parent.TokenCmd
}

// getSource populates project repos from a provider source.
func getSource(project *domain.Project, deps coreruntime.Runtime, o options) error {
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
	// A cache-only load never contacts a remote Provider Source: with no cache
	// to answer, the project keeps whatever repositories it was configured with
	// (none, for a discovered source) rather than triggering a network fetch or
	// a tokenCommand passphrase prompt. Shell completion loads this way. Local
	// filesystem discovery is offline, so it still runs.
	if o.cacheOnly && providers.IsRemote(source.Type) {
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
func loadFromProvider(project *domain.Project, deps coreruntime.Runtime) error {
	source := project.Source
	auth := deps.Settings.SourceAuth(source)
	// A bad providerTimeout falls back to its default here; the value is
	// validated and its warning surfaced once at startup (newRuntime), so the
	// error is intentionally dropped rather than reported again per fetch.
	timeout, _ := deps.Settings.ProviderTimeoutDuration()
	c, err := providers.NewGitProvider(deps.Ctx, source.Type, providers.Options{
		Token:           auth.Token,
		TokenCommand:    auth.Command(),
		IncludeArchived: deps.Settings.IncludeArchived,
		Log:             deps.Log,
		Timeout:         timeout,
		GitClient:       deps.Git,
	})
	if err != nil {
		return fmt.Errorf("failed to create provider: %w", err)
	}

	logger := logging.Or(deps.Log)
	if providers.IsRemote(source.Type) {
		logger.DebugContext(deps.Ctx, "fetching repos from provider",
			"provider", source.Type, "search", source.Search)
	} else {
		logger.DebugContext(deps.Ctx, "searching for repos on disk",
			"path", source.Search)
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
