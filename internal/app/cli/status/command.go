// Package status is the view side of `gits status`: it resolves what to run
// on, drives the probe, and renders the reports as a table or as JSON.
package status

import (
	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/app"
	"github.com/rafi/gits/internal/app/cli/output"
	"github.com/rafi/gits/internal/app/cli/pick"
	"github.com/rafi/gits/internal/app/cli/progress"
	"github.com/rafi/gits/internal/service/run"
	"github.com/rafi/gits/internal/service/status"
)

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

// keep reports whether rep matches at least one active filter: combined
// filters are a union, and rows with errors always stay visible.
func (o Options) keep(rep *status.Report) bool {
	if rep.Err != nil {
		return true
	}
	return o.Dirty && rep.Changed() || o.Unsynced && rep.Unsynced()
}

// rows indexes a run's reports by repository, so both renderers walk the
// project tree the run visited and look each repository's row up — the tree
// comes from the project, not from the order results arrived in.
type rows map[string]*status.Report

// newRows builds the index from the run's results. A repository the state
// guard turned back has no report; its row is built from the result.
func newRows(res run.Results[*status.Report]) rows {
	index := make(rows, len(res.Results))
	for _, r := range res.Results {
		rep := r.Value
		if rep == nil {
			rep = &status.Report{Repo: r.Repo, Err: r.Err}
		}
		index[r.Repo.Key()] = rep
	}
	return index
}

// visible returns one project's rows in its own repository order, dropping
// the repositories the run never started and those an active filter hides,
// plus the count it hid. Both renderers select rows through here so the JSON
// document holds exactly what the table would have shown.
func (r rows) visible(p domain.Project, opts Options) (reps []*status.Report, hidden int) {
	for _, repo := range p.Repos {
		rep, ok := r[repo.Key()]
		if !ok {
			continue // not started (canceled before dequeue)
		}
		if opts.filtered() && !opts.keep(rep) {
			hidden++
			continue
		}
		reps = append(reps, rep)
	}
	return reps, hidden
}

// ExecStatus displays the status of all repositories, as a compact table or
// as the JSON envelope it shares with `list`.
//
// Args: (optional)
//   - project name
//   - repo or sub-project name
func ExecStatus(format string, opts Options, args []string, deps app.RuntimeCLI) error {
	// Validate before anything is loaded or selected, so a typo'd format never
	// costs a provider round-trip or an interactive prompt. The accepted
	// formats are the ones every bulk command takes.
	if err := output.ValidateFormat(format); err != nil {
		return err
	}

	// What the command runs on is settled — prompting included — before the
	// engine is handed anything, so nothing it does can fail over an argument.
	target, err := pick.Target(args, deps)
	if err != nil {
		return err
	}

	res := run.Command[*status.Report]{
		Verb:     "checking status",
		Do:       status.Probe{Stat: opts.Stat}.Repo,
		Progress: progress.New(deps.Err),
	}.Run(target, deps.Runtime)
	output.Skips(res, deps)

	if format == output.FormatJSON {
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
	return output.Epilogue(res, deps)
}
