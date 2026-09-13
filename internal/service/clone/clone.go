// Package clone clones one repository, reporting git's output rather than a
// line to print.
package clone

import (
	"context"
	"errors"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/format"
	"github.com/rafi/gits/internal/infra/git"
	"github.com/rafi/gits/internal/service"
	"github.com/rafi/gits/internal/service/run"
)

// Report is what cloning one repository produced.
type Report struct {
	// Output is git's own output, unstyled.
	Output string
}

// Skip reports whether a project's configuration disables cloning it, and
// everything beneath it.
func Skip(p domain.Project) bool {
	return p.Clone != nil && !*p.Clone
}

// Accepts are the states clone is willing to receive: a repository not yet
// cloned is this command's expected input, where every other bulk command
// passes over it. Only a defective configuration has no destination to clone
// into.
func Accepts() []domain.RepoState {
	return []domain.RepoState{
		domain.RepoStateOK,
		domain.RepoStateNotCloned,
		domain.RepoStateRemoteOnly,
		domain.RepoStateUnknown,
	}
}

// Repo clones one repository. A repository already cloned, or one with
// nowhere to clone into, is a documented pass-over: a warning that shows on
// the repository's line without failing the run.
func Repo(ctx context.Context, repo run.Repo, rt service.Runtime) (Report, error) {
	// A remote-only repository is provider-backed with no local home, so
	// there is nothing to clone into. Pass over it with a warning that names
	// the config keys that would give it one, rather than handing git an
	// empty target.
	if repo.State == domain.RepoStateRemoteOnly {
		return Report{}, domain.NewWarning(
			"no local path: set `path:` on the project or `dir:` on the repository")
	}

	out, err := rt.Git.Clone(ctx, repo.Src, repo.AbsPath)
	if errors.Is(err, git.ErrTargetExists) {
		return Report{}, domain.NewWarning(
			"already cloned at %s", format.Path(repo.AbsPath, rt.HomeDir))
	}
	if err != nil {
		return Report{}, err
	}
	return Report{Output: out}, nil
}
