package catalog

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"sort"

	"github.com/mitchellh/go-homedir"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/infra/git"
	"github.com/rafi/gits/internal/infra/providers"
)

// expandPaths resolves each project's configured path to an absolute one, and
// gives every sub-project the path and source it inherits from its parent.
func expandPaths(ctx context.Context, logger *slog.Logger, project *domain.Project) {
	if project.Path != "" {
		var err error
		project.AbsPath, err = homedir.Expand(project.Path)
		if err != nil {
			logger.WarnContext(ctx, "unable to expand project path",
				"project", project.Name, "path", project.Path, "err", err)
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
		expandPaths(ctx, logger, sub)
	}
}

// classifyRepos determines the state of every repository in the tree.
func classifyRepos(ctx context.Context, project *domain.Project, git git.Reader) {
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
	git git.Reader,
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

	// A readable clone is `ok`. Its Repo Src is resolved lazily by FillSources,
	// only for the commands that display it: classification asking git for
	// every repository's remote URL here cost one subprocess per repository on
	// every command, even ones that never print the value.
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
		// Compare the display name, not the raw Name field: a repository
		// declared with only dir: or src: has an empty Name and would
		// otherwise sort as equal while named repositories sort alphabetically.
		// GetName is what every renderer and GetRepo lookup uses.
		return project.Repos[i].GetName() < project.Repos[j].GetName()
	})
}
