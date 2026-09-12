// Package cd implements `gits cd`, which prints a Repository's path for a
// shell to change into.
package cd

import (
	"fmt"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cli"
	"github.com/rafi/gits/internal/types"
)

// ExecCD returns the a repository path.
//
// Args: (optional)
//   - project name
//   - repo or sub-project name
func ExecCD(args []string, deps types.RuntimeCLI) error {
	_, repo, err := cli.ParseArgs(args, false, deps)
	if err != nil {
		return err
	}
	// Abort if repository is not cloned or has errors.
	if repo.State != domain.RepoStateOK {
		return cli.AbortOnRepoState(deps.Err, *repo, deps.Theme.Error)
	}

	fmt.Fprintln(deps.Out, repo.AbsPath)
	return nil
}
