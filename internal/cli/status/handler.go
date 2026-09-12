// Package status implements `gits status`, the Bulk Command that reports each
// Repository's work-tree and Upstream state.
package status

import (
	"context"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/bulk"
	"github.com/rafi/gits/internal/git"
	"github.com/rafi/gits/internal/types"
)

// repoStatus is one repository's row: the probe's answers, populated
// concurrently by statusRepo, or the error that stood in for them.
type repoStatus struct {
	// Snapshot is the one porcelain-v2 pass: branch, upstream, tracking
	// counts and work-tree counts.
	git.Snapshot

	repo bulk.Repo
	// stat is the uncommitted line diff vs HEAD; nil unless --stat asked for
	// it and the probe answered.
	stat *git.DiffStat
	// version is `git describe --tags` for HEAD, carried on the HeadInfo pass
	// (Head.Describe); empty when no tag is reachable, which is what the JSON
	// `version` field's absence means. Unlike the old `describe --always`, a
	// tagless repository is left blank rather than shown its bare hash.
	version string
	// head is the last commit; zero when HEAD is unborn.
	head git.Head
	// compared reports whether Ahead and Behind were measured — against the
	// Upstream, or against a same-named branch on one of the Remotes when no
	// Upstream resolves. False means both are zero for want of a reference.
	compared bool

	err error
}

// changed reports whether the work tree has any local changes.
func (s *repoStatus) changed() bool {
	return s.Staged+s.Unstaged+s.Untracked > 0
}

// unsynced reports whether the branch is ahead or behind its upstream. Repos
// without an upstream are not unsynced — there is nothing to sync with.
func (s *repoStatus) unsynced() bool {
	return s.Ahead > 0 || s.Behind > 0
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

// rows indexes a run's statuses by repository, so both renderers walk the
// project tree the run visited and look each repository's row up — the tree
// comes from the project, not from the order results arrived in.
type rows map[string]*repoStatus

// newRows builds the index from the run's results. A repository the state
// guard turned back has no probe value; its row is built from the result.
func newRows(res bulk.Results[*repoStatus]) rows {
	index := make(rows, len(res.Results))
	for _, r := range res.Results {
		st := r.Value
		if st == nil {
			st = &repoStatus{repo: r.Repo, err: r.Err}
		}
		index[r.Repo.Key()] = st
	}
	return index
}

// visible returns one project's rows in its own repository order, dropping
// the repositories the run never started and those an active filter hides,
// plus the count it hid. Both renderers select rows through here so the JSON
// document holds exactly what the table would have shown.
func (r rows) visible(p domain.Project, opts Options) (sts []*repoStatus, hidden int) {
	for _, repo := range p.Repos {
		st, ok := r[repo.Key()]
		if !ok {
			continue // not started (canceled before dequeue)
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

	res, err := bulk.Command[*repoStatus]{
		Verb: "checking status",
		Body: statusRepo(opts),
	}.Run(args, deps)
	if err != nil {
		return err
	}

	if format == "json" {
		// The json form prints no error epilogue and exits zero for
		// per-repository conditions: a repository's condition is data in the
		// document rather than the command's outcome. An interrupted run
		// still fails, because the document is incomplete and nothing inside
		// it says so.
		if err := renderJSON(deps.Out, res, opts); err != nil {
			return err
		}
		return res.Interrupted
	}
	renderTables(res, opts, deps)
	return bulk.Epilogue(res, deps)
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
		st := &repoStatus{repo: repo}

		// One porcelain-v2 pass covers worktree counts, branch and upstream
		// divergence. The Gone Upstream is read from it too rather than
		// paying for a second query; `pull` and `push` take no snapshot and
		// read the same state from a ref walk instead — see
		// git.HeadUpstream; a fix to one belongs in the other.
		snap, err := deps.Git.Snapshot(ctx, repo.AbsPath)
		if err != nil {
			st.err = err
			return st, err
		}
		st.Snapshot = snap

		if opts.Stat {
			// Tolerated like HeadInfo: an unborn HEAD leaves the column blank.
			if ds, err := deps.Git.WorkingDiff(ctx, repo.AbsPath); err == nil {
				st.stat = &ds
			}
		}

		switch {
		case snap.Tracking:
			st.compared = true
		default:
			// Nothing resolves to compare against — no Upstream configured, or
			// a Gone one: compare against the matching branch on one of the
			// repository's own Remotes, so a usable comparison is not
			// discarded.
			ref := deps.Git.FallbackRef(ctx, repo.AbsPath, snap.Branch)
			if ref == "" {
				break
			}
			if ahead, behind, err := deps.Git.Diff(ctx, repo.AbsPath, snap.Branch, ref); err == nil {
				st.Ahead, st.Behind, st.compared = ahead, behind, true
			}
		}

		if head, err := deps.Git.HeadInfo(ctx, repo.AbsPath); err == nil {
			st.head = head
			st.version = head.Describe
		}

		return st, nil
	}
}
