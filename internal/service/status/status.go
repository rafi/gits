package status

import (
	"context"

	"github.com/rafi/gits/internal/service"
	"github.com/rafi/gits/internal/service/run"
)

// Probe is what the status probe is asked to measure. Stat costs an extra
// git invocation per repository, so it is only run when asked for.
type Probe struct {
	Stat bool // also measure the uncommitted line diff vs HEAD
}

// Repo probes one repository into a structured report: safe to call
// concurrently and writes no output itself.
func (p Probe) Repo(
	ctx context.Context,
	repo run.Repo,
	rt service.Runtime,
) (*Report, error) {
	rep := &Report{Repo: repo}

	// One porcelain-v2 pass covers worktree counts, branch and upstream
	// divergence. The Gone Upstream is read from it too rather than paying
	// for a second query; `pull` and `push` take no snapshot and read the
	// same state from a ref walk instead — see git.HeadUpstream; a fix to one
	// belongs in the other.
	snap, err := rt.Git.Snapshot(ctx, repo.AbsPath)
	if err != nil {
		rep.Err = err
		return rep, err
	}
	rep.Snapshot = snap

	if p.Stat {
		// Tolerated like HeadInfo: an unborn HEAD leaves the column blank.
		if ds, err := rt.Git.WorkingDiff(ctx, repo.AbsPath); err == nil {
			rep.Stat = &ds
		}
	}

	switch {
	case snap.Tracking:
		rep.Compared = true
	default:
		// Nothing resolves to compare against — no Upstream configured, or a
		// Gone one: compare against the matching branch on one of the
		// repository's own Remotes, so a usable comparison is not discarded.
		ref := rt.Git.FallbackRef(ctx, repo.AbsPath, snap.Branch)
		if ref == "" {
			break
		}
		if ahead, behind, err := rt.Git.Diff(ctx, repo.AbsPath, snap.Branch, ref); err == nil {
			rep.Ahead, rep.Behind, rep.Compared = ahead, behind, true
		}
	}

	if head, err := rt.Git.HeadInfo(ctx, repo.AbsPath); err == nil {
		rep.Head = head
		rep.Version = head.Describe
	}

	return rep, nil
}
