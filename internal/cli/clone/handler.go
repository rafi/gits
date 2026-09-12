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
	res, err := bulk.Command[string]{
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
		Skip: skipped,
		Body: cloneRepo,
	}.Run(args, deps)
	if err != nil {
		return err
	}
	return bulk.Lines(res, deps)
}

// skipped reports whether a project's configuration disables cloning it, and
// everything beneath it.
func skipped(p domain.Project) bool {
	return p.Clone != nil && !*p.Clone
}

// cloneRepo clones one repository and returns its result line's body.
func cloneRepo(ctx context.Context, repo bulk.Repo, deps types.RuntimeCLI) (string, error) {
	// A remote-only repository is provider-backed with no local home, so there
	// is nothing to clone into. Pass over it with a warning that names the
	// config keys that would give it one, rather than handing git an empty
	// target.
	if repo.State == domain.RepoStateRemoteOnly {
		return "", types.NewWarning(
			"no local path: set `path:` on the project or `dir:` on the repository")
	}

	output, err := deps.Git.Clone(ctx, repo.Src, repo.AbsPath)
	if errors.Is(err, git.ErrTargetExists) {
		// Repo is already cloned, skip with a warning.
		repoPath := cli.Path(repo.AbsPath, deps.HomeDir)
		return "", types.NewWarning("already cloned at %s", repoPath)
	}
	if err != nil {
		return "", err
	}
	return deps.Theme.GitOutput.Render(output), nil
}
