// Package status implements `gits status`, the Bulk Command that reports each
// Repository's work-tree and Upstream state.
package status

import (
	"context"
	"errors"
	"time"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/bulk"
	"github.com/rafi/gits/internal/types"
)

// repoStatus is one repository's structured status, populated concurrently by
// the probe (statusRepo) and rendered as a table row by render.go.
type repoStatus struct {
	repo   domain.Repository
	title  string
	branch string

	staged    int
	unstaged  int
	untracked int

	added   int  // uncommitted line insertions vs HEAD (--stat)
	deleted int  // uncommitted line deletions vs HEAD (--stat)
	hasStat bool // the --stat diff probe answered; added/deleted are measured

	ahead  int
	behind int
	// noUpstream means neither an Upstream nor a same-named branch on any
	// Remote was available to compare against, so ahead/behind are zero for
	// want of a reference rather than because the branch is level.
	noUpstream bool
	// upstream is the configured Upstream's name, empty when the branch has
	// none. Independent of noUpstream, which is about the comparison.
	upstream string
	// upstreamGone marks the Gone Upstream: upstream is configured, but no
	// Remote branch is behind it any more.
	upstreamGone bool

	version string
	commit  string
	// message is the table's last column, and carries whichever of the two
	// things belongs there: the last commit's subject, or — when err is set —
	// the bare failure reason. Read it through err, never on its own.
	message string
	when    time.Time

	err error
}

// changed reports whether the work tree has any local changes.
func (s *repoStatus) changed() bool {
	return s.staged+s.unstaged+s.untracked > 0
}

// unsynced reports whether the branch is ahead or behind its upstream. Repos
// without an upstream are not unsynced — there is nothing to sync with.
func (s *repoStatus) unsynced() bool {
	return s.ahead > 0 || s.behind > 0
}

// Options are the status command's display flags.
type Options struct {
	Stat     bool // show the HEAD± column of uncommitted line diffs
	Dirty    bool // show only repos with uncommitted changes
	Unsynced bool // show only repos ahead or behind upstream
}

// filtered reports whether any row filter is active.
func (o Options) filtered() bool {
	return o.Dirty || o.Unsynced
}

// keep reports whether st matches at least one active filter: combined
// filters are a union, and rows with errors always stay visible.
func (o Options) keep(st *repoStatus) bool {
	if st.err != nil {
		return true
	}
	return o.Dirty && st.changed() || o.Unsynced && st.unsynced()
}

// visibleStatuses returns one group's statuses in stable tree order, dropping
// the repositories the run never started and those an active filter hides,
// plus the count it hid. Both renderers select rows through here so the JSON
// document holds exactly what the table would have shown.
func visibleStatuses(g bulk.Group[*repoStatus], opts Options) (sts []*repoStatus, hidden int) {
	for _, res := range g.Results {
		if res == nil {
			continue // not started (canceled before dequeue)
		}
		st := statusOf(res)
		if opts.filtered() && !opts.keep(st) {
			hidden++
			continue
		}
		sts = append(sts, st)
	}
	return sts, hidden
}

// statusOf returns the row a result carries, building one for a repository the
// state guard turned back before the probe could. The collected value is the
// probe's own type, so there is no assertion here to fail and no row to drop.
func statusOf(res *bulk.Result[*repoStatus]) *repoStatus {
	if res.Value != nil {
		return res.Value
	}
	return &repoStatus{
		repo:    res.Repo.Repository,
		title:   res.Repo.Title.Value(),
		err:     res.Err,
		message: reason(res.Err),
	}
}

// ExecStatus displays the status of all repositories, as a compact table or
// as the JSON envelope it shares with `list`.
//
// Args: (optional)
//   - project name
//   - repo or sub-project name
func ExecStatus(format string, opts Options, args []string, deps types.RuntimeCLI) error {
	// Validate before anything is loaded or selected, so a typo'd format never
	// costs a provider round-trip or an interactive prompt.
	if err := validateFormat(format); err != nil {
		return err
	}

	// Unlike the four line-rendering Bulk Commands, status collects a
	// structured result per repository, buffers the whole tree so table
	// columns can be sized, and emits two formats with different rules about
	// the exit code — so it supplies its own renderer instead of the stock one.
	return bulk.Command[*repoStatus]{
		Verb:   "checking status",
		Body:   statusRepo(opts),
		Render: render(format, opts),
	}.Run(args, deps)
}

// statusRepo returns a body that probes one repository into a structured
// status: safe to call concurrently and writes no output itself.
func statusRepo(opts Options) func(
	context.Context, bulk.Repo, types.RuntimeCLI,
) (*repoStatus, error) {
	return func(
		ctx context.Context,
		repo bulk.Repo,
		deps types.RuntimeCLI,
	) (*repoStatus, error) {
		st := &repoStatus{
			repo:  repo.Repository,
			title: repo.Title.Value(),
		}

		// One porcelain-v2 pass covers worktree counts, branch and upstream
		// divergence — state that previously took four git invocations.
		snap, err := deps.Git.Snapshot(ctx, repo.AbsPath)
		if err != nil {
			st.err = err
			st.message = reason(err)
			return st, err
		}
		st.staged, st.unstaged, st.untracked = snap.Staged, snap.Unstaged, snap.Untracked
		st.branch = snap.Branch
		// The snapshot is already taken, so the Gone Upstream is read from it
		// rather than paying for a second query. `pull` and `push` take no
		// snapshot and read the same state from a ref walk instead — see
		// git.HeadUpstream; a fix to one belongs in the other.
		st.upstream, st.upstreamGone = snap.Upstream, snap.GoneUpstream()

		if opts.Stat {
			// Tolerated like Describe: an unborn HEAD leaves the column blank.
			if ds, err := deps.Git.WorkingDiff(ctx, repo.AbsPath); err == nil {
				st.added, st.deleted, st.hasStat = ds.Added, ds.Deleted, true
			}
		}

		if version, err := deps.Git.Describe(ctx, repo.AbsPath); err == nil {
			st.version = version
		}

		if snap.Tracking {
			st.ahead, st.behind = snap.Ahead, snap.Behind
		} else if ref := deps.Git.FallbackRef(ctx, repo.AbsPath, st.branch); ref != "" {
			// Nothing resolves to compare against — no Upstream configured, or
			// a Gone one: compare against the matching branch on one of the
			// repository's own Remotes, so a usable comparison is not
			// discarded.
			if ahead, behind, err := deps.Git.Diff(ctx, repo.AbsPath, st.branch, ref); err == nil {
				st.ahead, st.behind = ahead, behind
			} else {
				st.noUpstream = true
			}
		} else {
			st.noUpstream = true
		}

		if head, err := deps.Git.HeadInfo(ctx, repo.AbsPath); err == nil {
			st.commit, st.message, st.when = head.Hash, head.Subject, head.Time
		}

		return st, nil
	}
}

// reason extracts the bare failure reason for the message cell, avoiding the
// repo-name/path repetition types.Warning.Error() adds for error listings.
func reason(err error) string {
	var w *types.Warning
	if errors.As(err, &w) && w.Reason != "" {
		return w.Reason
	}
	return err.Error()
}
