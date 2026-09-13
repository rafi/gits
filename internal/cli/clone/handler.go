// Package clone implements `gits clone`, the Bulk Command that clones every
// not-cloned Repository in a Project.
package clone

import (
	"context"
	"errors"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/app"
	"github.com/rafi/gits/internal/app/cli/output"
	"github.com/rafi/gits/internal/app/cli/pick"
	"github.com/rafi/gits/internal/app/cli/progress"
	"github.com/rafi/gits/internal/format"
	"github.com/rafi/gits/internal/git"
	"github.com/rafi/gits/internal/service"
	"github.com/rafi/gits/internal/service/run"
)

// ExecClone clones project repositories, or a specific repo, rendering the
// results as lines or as the JSON envelope.
//
// Args: (optional)
//   - project name
//   - repo or sub-project name
func ExecClone(format string, args []string, deps app.RuntimeCLI) error {
	// Validate before anything is loaded or selected, so a typo'd format never
	// costs a provider round-trip or an interactive prompt.
	if err := output.ValidateFormat(format); err != nil {
		return err
	}

	// What the command runs on is settled — prompting included — before the
	// engine is handed anything, so nothing it does can fail over an argument.
	target, err := pick.Target(args, deps)
	if err != nil {
		return err
	}

	res := run.Command[string]{
		Name: "clone",
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
		Skip:     skipped,
		Do:       cloneRepo(deps),
		Progress: progress.New(deps.Err),
	}.Run(target, deps.Runtime)
	return output.Render(res, format, output.PlainView, deps)
}

// skipped reports whether a project's configuration disables cloning it, and
// everything beneath it.
func skipped(p domain.Project) bool {
	return p.Clone != nil && !*p.Clone
}

// cloneRepo returns a body that clones one repository into its result line.
func cloneRepo(deps app.RuntimeCLI) func(
	context.Context, run.Repo, service.Runtime,
) (string, error) {
	return func(ctx context.Context, repo run.Repo, rt service.Runtime) (string, error) {
		// A remote-only repository is provider-backed with no local home, so
		// there is nothing to clone into. Pass over it with a warning that
		// names the config keys that would give it one, rather than handing
		// git an empty target.
		if repo.State == domain.RepoStateRemoteOnly {
			return "", domain.NewWarning(
				"no local path: set `path:` on the project or `dir:` on the repository")
		}

		out, err := rt.Git.Clone(ctx, repo.Src, repo.AbsPath)
		if errors.Is(err, git.ErrTargetExists) {
			// Repo is already cloned, skip with a warning.
			repoPath := format.Path(repo.AbsPath, rt.HomeDir)
			return "", domain.NewWarning("already cloned at %s", repoPath)
		}
		if err != nil {
			return "", err
		}
		return deps.Theme.GitOutput.Render(out), nil
	}
}
