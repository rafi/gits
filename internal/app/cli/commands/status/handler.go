// Package status is the view side of `gits status`: it resolves what to run
// on, drives the probe, and renders the reports as a table or as JSON.
package status

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/mitchellh/go-homedir"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/app"
	"github.com/rafi/gits/internal/app/cli/interaction/progress"
	pick "github.com/rafi/gits/internal/app/cli/interaction/select"
	"github.com/rafi/gits/internal/app/cli/render/output"
	"github.com/rafi/gits/internal/runtime/command"
	"github.com/rafi/gits/internal/runtime/projects"
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
func newRows(res command.Results[*status.Report]) rows {
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
func ExecStatus(
	format string, opts Options, tags domain.TagSet,
	args []string, deps app.RuntimeCLI,
) error {
	// Validate before anything is loaded or selected, so a typo'd format never
	// costs a provider round-trip or an interactive prompt. The accepted
	// formats are the ones every bulk command takes.
	if err := output.ValidateFormat(format); err != nil {
		return err
	}

	// What the command runs on is settled — prompting included — before the
	// engine is handed anything, so nothing it does can fail over an argument.
	target, ok, err := pathTarget(args, tags, deps)
	if err != nil {
		return err
	}
	if !ok {
		target, err = pick.TargetWithTags(args, tags, deps)
	}
	if err != nil {
		return err
	}

	res := command.Command[*status.Report]{
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

// pathTarget returns a live filesystem target for `gits status <path>`. Status
// is intentionally usable without a config entry: a path names the repository at
// that directory, or every repository underneath it; a file path names its
// containing repository. Bare words still prefer configured projects, so an
// existing directory does not shadow a project of the same name.
func pathTarget(args []string, tags domain.TagSet, deps app.RuntimeCLI) (command.Target, bool, error) {
	if len(args) != 1 {
		return command.Target{}, false, nil
	}
	_, configuredProject := deps.Projects[args[0]]
	if !isPath(args[0]) && configuredProject {
		return command.Target{}, false, nil
	}
	path, err := homedir.Expand(args[0])
	if err != nil {
		return command.Target{}, true, err
	}
	info, err := os.Stat(path)
	if err != nil {
		if isPath(args[0]) {
			return command.Target{}, true, err
		}
		return command.Target{}, false, nil
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return command.Target{}, true, err
	}
	if !info.IsDir() {
		path = containingRepo(path, deps)
	}
	project, err := projects.LoadOne(path, deps.Runtime, projects.WithTags(tags))
	if err != nil {
		return command.Target{}, true, err
	}
	// Repositories found by path carry no tags.
	if !tags.Empty() && project.CountRepos() == 0 {
		return command.Target{}, true, domain.NewWarning(
			"no repository under %s carries tag %s", args[0], tags)
	}
	return command.Target{Project: project}, true, nil
}

func isPath(arg string) bool {
	return arg == "." || strings.HasPrefix(arg, "./") || strings.HasPrefix(arg, "../") ||
		strings.HasPrefix(arg, "~/") || filepath.IsAbs(arg)
}

func containingRepo(path string, deps app.RuntimeCLI) string {
	dir := filepath.Dir(path)
	for {
		if deps.Git.IsRepo(deps.Ctx, dir) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return filepath.Dir(path)
		}
		dir = parent
	}
}
