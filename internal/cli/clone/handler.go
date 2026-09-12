// Package clone implements `gits clone`, the Bulk Command that clones every
// not-cloned Repository in a Project.
package clone

import (
	"context"
	"errors"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/bulk"
	"github.com/rafi/gits/internal/cli"
	"github.com/rafi/gits/internal/git"
	"github.com/rafi/gits/internal/types"
)

// ExecClone clones project repositories, or a specific repo.
//
// Args: (optional)
//   - project name
//   - repo or sub-project name
func ExecClone(args []string, deps types.RuntimeCLI) error {
	return bulk.Command[string]{
		Verb: "cloning",
		// A repository not yet cloned is this command's expected input, where
		// every other Bulk Command passes over it. Only a defective
		// configuration has no destination to clone into.
		Accepts: []domain.RepoState{
			domain.RepoStateOK,
			domain.RepoStateNotCloned,
			domain.RepoStateRemoteOnly,
			domain.RepoStateUnknown,
		},
		Skip:   skipped,
		Body:   cloneRepo,
		Render: bulk.Lines,
	}.Run(args, deps)
}

// skipped reports whether a project's configuration disables cloning it, and
// everything beneath it.
func skipped(p domain.Project) bool {
	return p.Clone != nil && !*p.Clone
}

// cloneRepo clones one repository and returns its result line's body: safe to
// call concurrently and never writes to a destination.
func cloneRepo(ctx context.Context, repo bulk.Repo, deps types.RuntimeCLI) (string, error) {
	output, err := deps.Git.Clone(ctx, repo.Src, repo.AbsPath)
	if errors.Is(err, git.ErrTargetExists) {
		// Already cloned is a condition this command passes over, so it is
		// wrapped here as a warning the module leaves alone: it shows on the
		// repository's line without failing the run.
		repoPath := cli.Path(repo.AbsPath, deps.HomeDir)
		return "", types.NewWarning("already cloned at %s", repoPath)
	}
	if err != nil {
		return "", err
	}
	return deps.Theme.GitOutput.Render(output), nil
}
