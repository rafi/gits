package browse

import (
	"strings"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cli"
	"github.com/rafi/gits/internal/cli/types"
)

// ExecBrowse opens a fzf window to browse the entire catalog.
// Args: (optional)
//   - project name
//   - repo or sub-project name
//   - branch name
func ExecBrowse(args []string, deps types.RuntimeDeps) error {
	project, err := cli.GetOrSelectProject(args, deps)
	if err != nil {
		return err
	}

	if len(args) > 1 && strings.Index(args[1], "/") > 0 {
		args = args[:len(args)-1]
	}

	repo, repoName, err := cli.GetOrSelectRepo(project, args, deps)
	if err != nil {
		return err
	}

	// Abort if repository is not cloned or has errors.
	if repo.State != domain.RepoStateOK {
		return cli.AbortOnRepoState(repo, deps.Theme)
	}

	branchName := ""
	if len(args) > 2 {
		branchName = args[2]
	} else if len(args) < 3 {
		branchName, err = cli.SelectBranch(project.Name, repoName, repo, deps)
		if err != nil {
			return err
		}
	}
	args = []string{project.Name, repoName, branchName}
	return ExecBranchOverview(args, deps)
}
