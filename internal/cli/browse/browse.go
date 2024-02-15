package browse

import (
	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cli"
	"github.com/rafi/gits/internal/types"
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
		return cli.AbortOnRepoState(*repo, deps.Theme)
	}

	// Use the project name if provided, and branch too.
	projName := ""
	repoFullName := repo.GetNameWithNamespace()
	branch := ""
	switch len(args) {
	case 3:
		branch = args[2]
	case 0:
		projName = project.Name
	default:
		projName = args[0]
	}

	if branch == "" {
		// Interactively select a branch.
		branch, err = cli.SelectBranch(projName, *repo, deps)
		if err != nil {
			return err
		}
	}
	return ExecBranchOverview([]string{projName, repoFullName, branch}, deps)
}
