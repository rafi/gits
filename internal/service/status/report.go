// Package status probes one repository's work-tree and upstream state,
// reporting the answers rather than a rendered row.
package status

import (
	"github.com/rafi/gits/internal/git"
	"github.com/rafi/gits/internal/service/run"
)

// Report is one repository's probed state, or the error that stood in for it.
type Report struct {
	// Snapshot is the one porcelain-v2 pass: branch, upstream, tracking
	// counts and work-tree counts.
	git.Snapshot

	// Repo is the repository the probe ran on, with the display path the
	// engine derived for it.
	Repo run.Repo
	// Stat is the uncommitted line diff vs HEAD; nil unless the probe was
	// asked for it and git answered.
	Stat *git.DiffStat
	// Version is `git describe --tags` for HEAD, carried on the HeadInfo pass
	// (Head.Describe); empty when no tag is reachable, which is what the JSON
	// `version` field's absence means. Unlike the old `describe --always`, a
	// tagless repository is left blank rather than shown its bare hash.
	Version string
	// Head is the last commit; zero when HEAD is unborn.
	Head git.Head
	// Compared reports whether Ahead and Behind were measured — against the
	// Upstream, or against a same-named branch on one of the Remotes when no
	// Upstream resolves. False means both are zero for want of a reference.
	Compared bool

	// Err is the failure that replaced the probe's answers.
	Err error
}

// Changed reports whether the work tree has any local changes.
func (r *Report) Changed() bool {
	return r.Staged+r.Unstaged+r.Untracked > 0
}

// Unsynced reports whether the branch is ahead or behind its upstream. Repos
// without an upstream are not unsynced — there is nothing to sync with.
func (r *Report) Unsynced() bool {
	return r.Ahead > 0 || r.Behind > 0
}
