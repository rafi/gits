package status

import (
	"context"
	"errors"
	"time"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cli"
	"github.com/rafi/gits/internal/cli/walk"
	"github.com/rafi/gits/internal/types"
)

// repoStatus is one repository's structured status, populated concurrently by
// the walker (statusRepo) and rendered as a table row by render.go.
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
// the repositories the walker never started and those an active filter hides,
// plus the count it hid. Both renderers select rows through here so the JSON
// document holds exactly what the table would have shown.
func visibleStatuses(g walk.GroupResult, opts Options) (sts []*repoStatus, hidden int) {
	for _, res := range g.Results {
		if res == nil {
			continue // not started (cancelled before dequeue)
		}
		st, ok := res.Payload.(*repoStatus)
		if !ok {
			continue
		}
		if opts.filtered() && !opts.keep(st) {
			hidden++
			continue
		}
		sts = append(sts, st)
	}
	return sts, hidden
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

	project, repo, err := cli.ParseArgs(args, true, deps)
	if err != nil {
		return err
	}
	probe := statusRepo(opts)

	if repo != nil {
		// Single repository: a one-row table without a project title.
		groups, err := walk.SingleCollect(deps.Ctx, project, *repo, deps, probe)
		if format == "json" {
			// SingleCollect's only error is this one repository's, and that
			// is already in the document — as its state, or as the nested
			// status object's error. Returning it too would contradict the
			// envelope, where a repository's condition is data rather than
			// the command's outcome.
			return renderJSON(deps.Out, groups, false, opts)
		}
		renderGroups(groups, false, opts, deps)
		return err
	}

	// Collect every repository's structured status through the shared walker,
	// then render the whole tree at once so table columns can be sized.
	groups, interrupted := walk.Collect(deps.Ctx, project, deps, "checking status", probe)
	if format == "json" {
		// An interruption still fails: that document is incomplete, and
		// nothing inside it says so.
		if err := renderJSON(deps.Out, groups, true, opts); err != nil {
			return err
		}
		return interrupted
	}
	errs := renderGroups(groups, true, opts, deps)
	return cli.RenderErrors(deps.Err, walk.WithInterrupted(errs, interrupted), true)
}

// statusRepo returns a walk.RepoFunc that probes one repository into a
// structured status: safe to call concurrently and writes no output itself.
func statusRepo(opts Options) walk.RepoFunc {
	return func(
		ctx context.Context,
		project domain.Project,
		repo domain.Repository,
		deps types.RuntimeCLI,
	) walk.RepoResult {
		st := &repoStatus{
			repo:  repo,
			title: cli.RepoRelPath(project, repo, deps.HomeDir),
		}

		// Abort if repository is not cloned or has errors.
		if repo.State != domain.RepoStateOK {
			err := cli.RepoStateError(repo)
			st.err = err
			st.message = reason(err)
			return walk.RepoResult{Payload: st, Err: err}
		}

		// One porcelain-v2 pass covers worktree counts, branch and upstream
		// divergence — state that previously took four git invocations.
		snap, err := deps.Git.Snapshot(ctx, repo.AbsPath)
		if err != nil {
			st.err = err
			st.message = reason(err)
			return walk.RepoResult{Payload: st, Err: cli.RepoError(err, repo)}
		}
		st.staged, st.unstaged, st.untracked = snap.Staged, snap.Unstaged, snap.Untracked
		st.branch = snap.Branch

		if opts.Stat {
			// Tolerated like Describe: an unborn HEAD leaves the column blank.
			if ds, err := deps.Git.WorkingDiff(ctx, repo.AbsPath); err == nil {
				st.added, st.deleted, st.hasStat = ds.Added, ds.Deleted, true
			}
		}

		if version, err := deps.Git.Describe(ctx, repo.AbsPath); err == nil {
			st.version = version
		}

		if snap.HasUpstream {
			st.ahead, st.behind = snap.Ahead, snap.Behind
		} else if ref := deps.Git.FallbackRef(ctx, repo.AbsPath, st.branch); ref != "" {
			// No upstream configured: compare against the matching branch on
			// the repo's actual remote.
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

		return walk.RepoResult{Payload: st}
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
