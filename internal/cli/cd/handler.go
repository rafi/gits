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
	if repo == nil {
		return fmt.Errorf("missing repo name")
	}

	// Abort if repository is not cloned or has errors.
	if repo.State != domain.RepoStateOK {
		return cli.AbortOnRepoState(*repo, deps.Theme.Error)
	}

	fmt.Println(repo.AbsPath)
	return nil
}
