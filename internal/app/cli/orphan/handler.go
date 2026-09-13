// Package orphan implements `gits orphan`, which lists repositories on disk
// under a Project Path that the Project's config does not declare.
package orphan

import (
	"charm.land/lipgloss/v2"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/app"
	"github.com/rafi/gits/internal/app/cli/pick"
	"github.com/rafi/gits/internal/app/cli/style"
	"github.com/rafi/gits/internal/format"
	"github.com/rafi/gits/internal/service/orphan"
)

// ExecOrphan lists orphaned repositories, ones the project's config does not
// declare. With a repo argument, the scan is scoped to that repository's work
// tree and reports the git repositories embedded inside it.
//
// Args: (optional)
//   - project name
//   - repo or sub-project name
func ExecOrphan(args []string, deps app.RuntimeCLI) error {
	project, repo, err := pick.ParseArgs(args, true, deps)
	if err != nil {
		return err
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

	errorStyle := deps.Theme.Error.MarginLeft(style.LeftMargin)
	lipgloss.Fprintln(deps.Out, style.ProjectTitleWithBullet(project, deps.Theme))
	for _, repo := range repos {
		repoDir := format.Path(repo.Dir, deps.HomeDir)
		lipgloss.Fprintf(deps.Out, "%s - %s\n", errorStyle.Render(repoDir), repo.Src)
	}
	return nil
}
