// Package orphan implements `gits orphan`, which lists repositories on disk
// under a Project Path that the Project's config does not declare.
package orphan

import (
	"charm.land/lipgloss/v2"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/app"
	pick "github.com/rafi/gits/internal/app/cli/interaction/select"
	"github.com/rafi/gits/internal/app/cli/render/style"
	"github.com/rafi/gits/internal/format"
	"github.com/rafi/gits/internal/service/orphan"
)

// ExecOrphan lists orphaned repositories, ones the project's config does not
// declare. With a repo argument, the scan is scoped to that repository's work
// tree and reports the git repositories embedded inside it.
//
// A `--label` scans the work tree of each labeled repository.
//
// Args: (optional)
//   - project name
//   - repo or sub-project name
func ExecOrphan(labels domain.LabelSet, args []string, deps app.RuntimeCLI) error {
	project, repo, err := pick.ParseArgsWithLabels(args, labels, true, deps)
	if err != nil {
		return err
	}

	if repo == nil && !labels.Empty() {
		return orphansInLabeled(project, deps)
	}

	var repos []domain.Repository
	if repo != nil {
		if repo.State != domain.RepoStateOK {
			return style.AbortOnRepoState(deps.Err, *repo, deps.Theme.Error)
		}
		repos, err = orphan.InWorkTree(repo.AbsPath, deps.Runtime)
	} else {
		repos, err = orphan.InProject(project, deps.Runtime)
	}
	if err != nil {
		return err
	}

	lipgloss.Fprintln(deps.Out, style.ProjectTitleWithBullet(project, deps.Theme))
	renderRepos(repos, deps)
	return nil
}

// orphansInLabeled scans the work tree of every repository in project.
// Unreadable repositories are reported and skipped.
func orphansInLabeled(project domain.Project, deps app.RuntimeCLI) error {
	lipgloss.Fprintln(deps.Out, style.ProjectTitleWithBullet(project, deps.Theme))
	return walkLabeled(project, deps)
}

// walkLabeled scans each repository in Traversal order.
func walkLabeled(project domain.Project, deps app.RuntimeCLI) error {
	for _, repo := range project.Repos {
		if repo.State != domain.RepoStateOK {
			if err := style.AbortOnRepoState(deps.Err, repo, deps.Theme.Error); err != nil &&
				!domain.IsWarning(err) {
				return err
			}
			continue
		}
		repos, err := orphan.InWorkTree(repo.AbsPath, deps.Runtime)
		if err != nil {
			return err
		}
		renderRepos(repos, deps)
	}
	for _, sub := range project.SubProjects {
		if err := walkLabeled(sub, deps); err != nil {
			return err
		}
	}
	return nil
}

// renderRepos prints each orphaned repository's directory and remote.
func renderRepos(repos []domain.Repository, deps app.RuntimeCLI) {
	errorStyle := deps.Theme.Error.MarginLeft(style.LeftMargin)
	for _, repo := range repos {
		repoDir := format.Path(repo.Dir, deps.HomeDir)
		lipgloss.Fprintf(deps.Out, "%s - %s\n", errorStyle.Render(repoDir), repo.Src)
	}
}
