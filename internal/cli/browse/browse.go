package browse

import (
	"fmt"

	"charm.land/lipgloss/v2"
	log "github.com/sirupsen/logrus"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cli"
	"github.com/rafi/gits/internal/loader"
	"github.com/rafi/gits/internal/types"
	"github.com/rafi/gits/pkg/fzf"
)

// ExecBrowse opens a fzf window to browse the entire catalog.
// Args: (optional)
//   - project name
//   - repo or sub-project name
//   - branch name
func ExecBrowse(args []string, deps types.RuntimeCLI) error {
	project, repo, err := cli.ParseArgs(args, false, deps)
	if err != nil {
		return err
	}

	// Abort if repository is not cloned or has errors.
	if repo.State != domain.RepoStateOK {
		return cli.AbortOnRepoState(*repo, deps.Theme.Error)
	}

	// Use the project name if provided, and branch too.
	repoFullName := repo.GetNameWithNamespace()
	projName, branch := browseTarget(args, project)

	if branch == "" {
		// Interactively select a branch.
		branch, err = cli.SelectBranch(projName, *repo, deps)
		if err != nil {
			return err
		}
	}
	return renderBranchOverview(*repo, repoFullName, branch, deps)
}

// resolveProjectRepo loads the project and repository named by the first two
// arguments of a preview sub-command.
func resolveProjectRepo(args []string, deps types.RuntimeCLI) (domain.Repository, error) {
	if len(args) < 1 {
		return domain.Repository{}, fmt.Errorf("missing project name")
	}
	project, err := loader.GetProject(args[0], deps.Runtime)
	if err != nil {
		return domain.Repository{}, fmt.Errorf("unable to load project %q: %w", args[0], err)
	}
	if len(args) < 2 {
		return domain.Repository{}, fmt.Errorf("missing repo name")
	}
	repo, found := project.GetRepo(args[1], "")
	if !found {
		return domain.Repository{}, fmt.Errorf("repo %s/%s not found", args[0], args[1])
	}
	return repo, nil
}

// previewWidth returns the fzf preview pane width, or zero when unknown.
// Fzf sets environment variables to detect width/height, see man fzf.
func previewWidth() int {
	width, _, err := fzf.GetPreviewSize()
	if err != nil {
		log.Warnf("unable to parse FZF_PREVIEW_COLUMNS: %s", err)
	}
	return width
}

// previewHeader centers a preview pane header across its width.
func previewHeader(style lipgloss.Style, width int) lipgloss.Style {
	return style.Align(lipgloss.Center).Width(width - 2)
}

// browseTarget resolves the project name and explicit branch from the
// command arguments. The project name comes from the first argument whenever
// one is given — including the 3-arg form — and otherwise from the
// interactively selected project. An empty branch means "select one".
func browseTarget(args []string, project domain.Project) (projName, branch string) {
	projName = project.Name
	if len(args) > 0 {
		projName = args[0]
	}
	if len(args) == 3 {
		branch = args[2]
	}
	return projName, branch
}
