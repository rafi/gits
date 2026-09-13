// Package pull fast-forwards one repository, reporting what it found rather
// than what to print: the branch, its upstream and git's own output.
package pull

import (
	"context"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/infra/git"
	"github.com/rafi/gits/internal/service"
	"github.com/rafi/gits/internal/service/run"
)

// Report is what pulling one repository produced.
type Report struct {
	// Branch is the branch that was pulled, and Upstream the ref it was
	// pulled from. Both are empty when nothing was pulled.
	Branch   string
	Upstream string
	// Output is git's own output, unstyled.
	Output string
}

// Repo pulls one repository. A branch with nowhere to pull from is a
// documented pass-over, returned as a warning so it shows on the
// repository's line without failing the run.
func Repo(ctx context.Context, repo run.Repo, rt service.Runtime) (Report, error) {
	head, err := rt.Git.HeadUpstream(ctx, repo.AbsPath)
	if err != nil {
		return Report{}, err
	}

	switch {
	case head.Upstream == "":
		// There is nowhere to pull from.
		return Report{}, domain.NewWarning("skipped: %s", git.ErrNoUpstream)
	case head.Gone:
		// Branch is gone, merged and deleted?
		return Report{}, domain.NewWarning("skipped: %s: %s", head.Upstream, git.ErrUpstreamGone)
	}

	out, err := rt.Git.Pull(ctx, repo.AbsPath)
	if err != nil {
		return Report{}, err
	}
	return Report{Branch: head.Branch, Upstream: head.Upstream, Output: out}, nil
}
