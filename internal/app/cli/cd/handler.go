// Package cd implements `gits cd`, which prints a Repository's path for a
// shell to change into.
package cd

import (
	"fmt"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/app"
	"github.com/rafi/gits/internal/app/cli/pick"
	"github.com/rafi/gits/internal/app/cli/style"
)

// ExecCD returns the a repository path.
//
// Args: (optional)
//   - project name
//   - repo or sub-project name
func ExecCD(args []string, deps app.RuntimeCLI) error {
	_, repo, err := pick.ParseArgs(args, false, deps)
	if err != nil {
		return err
	}
	// Abort if repository is not cloned or has errors.
	if repo.State != domain.RepoStateOK {
		return style.AbortOnRepoState(deps.Err, *repo, deps.Theme.Error)
	}

	fmt.Fprintln(deps.Out, repo.AbsPath)
	return nil
}
