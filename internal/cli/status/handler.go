package status

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
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

	added   int // uncommitted line insertions vs HEAD (--stat)
	deleted int // uncommitted line deletions vs HEAD (--stat)

	ahead      int
	behind     int
	noUpstream bool

	version string
	commit  string
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

// ExecStatus displays a compact status table of all repositories.
//
// Args: (optional)
//   - project name
//   - repo or sub-project name
func ExecStatus(opts Options, args []string, deps types.RuntimeCLI) error {
	project, repo, err := cli.ParseArgs(args, true, deps)
	if err != nil {
		return err
	}
	probe := statusRepo(opts)

	if repo != nil {
		// Single repository: a one-row table without a project title.
		res := probe(deps.Ctx, project, *repo, deps)
		groups := []walk.GroupResult{
			{Project: project, Results: []*walk.RepoResult{&res}},
		}
		renderGroups(os.Stdout, os.Stderr, groups, false, opts, deps)
		return res.Err
	}

	// Collect every repository's structured status through the shared walker,
	// then render the whole tree at once so table columns can be sized.
	groups, interrupted := walk.Collect(deps.Ctx, project, deps, "checking status", probe)
	errs := renderGroups(os.Stdout, os.Stderr, groups, true, opts, deps)
	if interrupted != nil {
		errs = append(errs, interrupted)
	}
	return cli.RenderErrors(errs, true)
}

// statusRepo returns a walk.RepoFunc that probes one repository into a
// structured status: safe to call concurrently and never writes to stdout.
func statusRepo(opts Options) walk.RepoFunc {
	return func(
		ctx context.Context,
		project domain.Project,
		repo domain.Repository,
		deps types.RuntimeCLI,
	) walk.RepoResult {
		st := &repoStatus{
			repo:  repo,
			title: repoTitleText(repo, project, deps.HomeDir),
		}

		// Abort if repository is not cloned or has errors.
		if repo.State != domain.RepoStateOK {
			err := cli.RepoStateWarning(repo)
			st.err = err
			st.message = reason(err)
			return walk.RepoResult{Payload: st, Err: err}
		}

		wt, err := deps.Git.WorkingState(ctx, repo.AbsPath)
		if err != nil {
			st.err = err
			st.message = reason(err)
			return walk.RepoResult{Payload: st, Err: cli.RepoError(err, repo)}
		}
		st.staged, st.unstaged, st.untracked = wt.Staged, wt.Unstaged, wt.Untracked

		if opts.Stat {
			// Tolerated like Describe: an unborn HEAD leaves the column blank.
			if ds, err := deps.Git.WorkingDiff(ctx, repo.AbsPath); err == nil {
				st.added, st.deleted = ds.Added, ds.Deleted
			}
		}

		if version, err := deps.Git.Describe(ctx, repo.AbsPath); err == nil {
			st.version = version
		}

		st.branch, _ = deps.Git.CurrentBranch(ctx, repo.AbsPath)
		upstream, err := deps.Git.UpstreamBranch(ctx, repo.AbsPath)
		if err != nil || upstream == "" {
			// No upstream configured: compare against the conventional remote branch.
			upstream = fmt.Sprintf("origin/%v", st.branch)
		}
		if ahead, behind, err := deps.Git.Diff(ctx, repo.AbsPath, st.branch, upstream); err != nil {
			st.noUpstream = true
		} else {
			st.ahead, st.behind = ahead, behind
		}

		if head, err := deps.Git.HeadInfo(ctx, repo.AbsPath); err == nil {
			st.commit, st.message, st.when = head.Hash, head.Subject, head.Time
		}

		return walk.RepoResult{Payload: st}
	}
}

// repoTitleText is the plain-text repo title: the repo directory relative to
// its project, with ~ for the home directory (same identity as cli.RepoTitle,
// without styling).
func repoTitleText(repo domain.Repository, project domain.Project, homeDir string) string {
	repoPath := repo.Dir
	if repoPath == "" {
		repoPath = repo.AbsPath
	}
	repoPath = strings.TrimPrefix(repoPath, project.AbsPath+"/")
	return cli.Path(repoPath, homeDir)
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
