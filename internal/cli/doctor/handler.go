// Package doctor implements `gits doctor`, which reports configuration and
// environment problems that no other command is in a position to mention.
//
// Everything here is a check a normal run cannot make. A command loads the
// projects it was asked about and says what it found; it has no reason to
// notice that some *other* project's `path:` does not exist, that the cache
// holds an entry no version of gits will read again, or that `fzf` is missing
// on a machine where nothing interactive has been run yet. `doctor` asks all
// of those at once, so the answer to "why is gits behaving strangely" is one
// command rather than a hunt.
//
// It never contacts a Provider Source. A diagnostic that waits on the network
// or prompts for a passphrase is one people stop running, and every check
// here is answerable from the config file, the filesystem and PATH.
package doctor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/mitchellh/go-homedir"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/bulk"
	"github.com/rafi/gits/internal/cache"
	"github.com/rafi/gits/internal/cli/jsonout"
	"github.com/rafi/gits/internal/types"
)

// Level is how much a finding matters. It is wire vocabulary — the `-o json`
// document carries these strings — so the three are a contract, and display
// layers map them to color rather than the reverse.
type Level string

const (
	// LevelError is a finding that stops something from working. Any one of
	// them exits non-zero, so a CI job can gate on `gits doctor`.
	LevelError Level = "error"
	// LevelWarning is a finding that degrades gits without stopping it: a
	// missing finder leaves every non-interactive command working.
	LevelWarning Level = "warning"
	// LevelInfo is context rather than a defect — where the cache lives, what
	// git version answered. Reported because the questions this command
	// exists to answer are usually asked with these values in hand.
	LevelInfo Level = "info"
)

// Finding is one thing doctor has to say. Subject names what was inspected —
// a config key, a binary, a project — so findings stay greppable and sort
// into a stable order; Message is the prose.
type Finding struct {
	Level   Level  `json:"level"`
	Subject string `json:"subject"`
	Message string `json:"message"`
}

// Report is everything doctor found, in the order it checked.
type Report struct {
	ConfigPath string    `json:"configPath"`
	Findings   []Finding `json:"findings"`
}

// HasErrors reports whether any finding is an error, which is what decides the
// exit code.
func (r Report) HasErrors() bool {
	for _, f := range r.Findings {
		if f.Level == LevelError {
			return true
		}
	}
	return false
}

// ExecDoctor runs every check and renders the report.
//
// It takes no project argument by design: "which of my projects is
// misconfigured" is the question, so scoping the answer to a project the user
// already suspects would defeat it.
func ExecDoctor(format string, _ []string, deps types.RuntimeCLI) error {
	// doctor is not a Bulk Command, but it renders the same two formats, so
	// it shares the one validator rather than keeping a second copy of the
	// pair that could drift from the flag's completion and help text.
	if err := bulk.ValidateFormat(format); err != nil {
		return err
	}

	report := Check(deps)
	if err := render(format, report, deps); err != nil {
		return err
	}
	if report.HasErrors() {
		// The message is the report itself, which the user is looking at.
		// Returning a bare non-zero rather than an epilogue keeps the last
		// line of output a finding rather than a restatement.
		return types.ErrSilent
	}
	return nil
}

// Check runs every diagnostic and returns the findings. Exported so a test can
// assert on the findings themselves rather than on rendered text.
func Check(deps types.RuntimeCLI) Report {
	report := Report{ConfigPath: deps.ConfigPath}
	add := func(level Level, subject, format string, args ...any) {
		report.Findings = append(report.Findings, Finding{
			Level:   level,
			Subject: subject,
			Message: fmt.Sprintf(format, args...),
		})
	}

	checkConfig(deps, add)
	checkProjects(deps, add)
	checkBinaries(deps, add)
	checkCache(deps, add)
	return report
}

// addFunc records one finding. Passed to each check so they compose without
// each returning and merging its own slice.
type addFunc func(level Level, subject, format string, args ...any)

// checkConfig reports the config file in use, and the warnings gathered while
// loading it — unknown keys among them. Those are already surfaced on every
// run; repeating them here is deliberate, since this is the command someone
// runs when they want everything at once.
func checkConfig(deps types.RuntimeCLI, add addFunc) {
	if deps.ConfigPath == "" {
		add(LevelWarning, "config",
			"no config file found; gits is running with an empty configuration")
		return
	}
	add(LevelInfo, "config", "using %s", deps.ConfigPath)

	for _, w := range deps.ConfigWarnings {
		add(LevelError, "config", "%s", w)
	}
}

// checkProjects inspects each configured project without loading any of them:
// a Provider Source is never contacted, so this stays offline and prompt-free.
func checkProjects(deps types.RuntimeCLI, add addFunc) {
	if len(deps.Projects) == 0 {
		add(LevelWarning, "projects", "no projects are configured")
		return
	}

	for _, name := range deps.Projects.SortedNames() {
		project := deps.Projects[name]
		project.Name = name
		checkProject(project, name, add)
	}
}

// checkProject inspects one project and its sub-projects. subject is the
// project's path through the tree, so a finding names `acme/tools` rather than
// an ambiguous `tools`.
func checkProject(project domain.Project, subject string, add addFunc) {
	checkProjectPath(project, subject, add)

	for _, repo := range project.Repos {
		checkRepo(project, repo, subject+"."+repo.GetName(), add)
	}

	for _, sub := range project.SubProjects {
		// A sub-project with no path of its own sits beneath its parent, so
		// it inherits a path that exists and has nothing to report.
		if sub.Path == "" {
			sub.Path = project.Path
		}
		checkProject(sub, subject+"/"+sub.Name, add)
	}
}

// checkRepo reports a repository the configuration gives no usable local home.
// The loader already classifies both cases as Repo State `error`, but only
// once a project is loaded and only for the project the user happened to ask
// about. As a *config* problem — one of several projects being wrong, in a
// file the user is looking at — neither has ever been reported anywhere.
//
// A provider-backed project is exempt from both: its repositories legitimately
// have no local home until they are cloned.
func checkRepo(project domain.Project, repo domain.Repository, subject string, add addFunc) {
	if project.Source != nil && project.Source.Type != "" {
		return
	}
	switch {
	case repo.Dir == "" && project.Path == "":
		add(LevelError, subject,
			"no `path:` on the project and no `dir:` on the repository, so there is nowhere for it to live")
	case repo.Dir != "" && project.Path == "" && !filepath.IsAbs(expandHome(repo.Dir)):
		// A relative `dir:` is resolved against the Project Path, and without
		// one it would silently resolve against the working directory.
		add(LevelError, subject,
			"relative `dir: %s` needs the project to set `path:`", repo.Dir)
	}
}

// checkProjectPath reports a `path:` that names nothing on disk. A project
// whose repositories all carry an absolute `dir:` legitimately has no path at
// all, so only a path that was set and does not resolve is a finding.
func checkProjectPath(project domain.Project, subject string, add addFunc) {
	if project.Path == "" {
		return
	}
	path := expandHome(project.Path)
	info, err := os.Stat(path)
	switch {
	case os.IsNotExist(err):
		// Not an error: `gits clone` creates it. Saying so is the point —
		// otherwise an empty listing looks like a broken config.
		add(LevelWarning, subject,
			"`path: %s` does not exist yet; `gits clone` will create it", project.Path)
	case err != nil:
		add(LevelError, subject, "`path: %s` cannot be read: %v", project.Path, err)
	case !info.IsDir():
		add(LevelError, subject, "`path: %s` is not a directory", project.Path)
	}
}

// versionProbeTimeout bounds asking a binary for its version. Generous for a
// local process, short enough that a wedged one cannot hang the report.
const versionProbeTimeout = 5 * time.Second

// checkBinaries reports the external programs gits shells out to. git is
// required by every command that touches a repository; the finder is needed
// only to pick something interactively, so its absence is a warning.
func checkBinaries(deps types.RuntimeCLI, add addFunc) {
	if path, err := exec.LookPath("git"); err != nil {
		add(LevelError, "git", "not found on PATH; every repository operation will fail")
	} else {
		add(LevelInfo, "git", "%s (%s)", binaryVersion(deps.Ctx, "git", "--version"), path)
	}

	finder := deps.Settings.Finder.Binary
	subject := "finder"
	if finder == "" {
		finder = "fzf"
	} else {
		subject = "finder (settings.finder.binary)"
	}
	if path, err := exec.LookPath(finder); err != nil {
		add(LevelWarning, subject,
			"%q not found on PATH; interactive selection is unavailable", finder)
	} else {
		add(LevelInfo, subject, "%s (%s)", binaryVersion(deps.Ctx, finder, "--version"), path)
	}
}

// binaryVersion returns the first line the program prints for its version
// flag, or its bare name when it cannot be asked. It is reported as context,
// so a program that answers oddly is never a finding of its own.
//
// It runs under the command's context, and under a timeout of its own: a
// diagnostic that hangs because the thing it is diagnosing hangs is the one
// failure mode this command cannot afford.
func binaryVersion(ctx context.Context, name string, args ...string) string {
	ctx, cancel := context.WithTimeout(ctx, versionProbeTimeout)
	defer cancel()

	// name is either a fixed binary or the user's own configured finder,
	// which they are equally free to run themselves.
	out, err := exec.CommandContext(ctx, name, args...).Output()
	if err != nil {
		return name
	}
	line, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n")
	if line == "" {
		return name
	}
	return line
}

// checkCache reports where cache entries live and how each one stands against
// cacheTTL — the answer to "why is gits still showing a repository I deleted
// last week", which is otherwise invisible.
func checkCache(deps types.RuntimeCLI, add addFunc) {
	if deps.Settings.Cache != nil && !*deps.Settings.Cache {
		add(LevelInfo, "cache", "disabled by `settings.cache: false`")
		return
	}

	dir, err := cache.Dir()
	if err != nil {
		add(LevelError, "cache", "cannot locate the cache directory: %v", err)
		return
	}
	add(LevelInfo, "cache", "directory %s", dir)

	// An unparseable cacheTTL falls back to its default and is already
	// warned about at startup, so the fallback is used here without a second
	// complaint.
	ttl, _ := deps.Settings.CacheTTLDuration()
	entries, err := cache.Entries(ttl)
	if err != nil {
		add(LevelError, "cache", "cannot read the cache directory: %v", err)
		return
	}
	if len(entries) == 0 {
		add(LevelInfo, "cache", "no projects are cached yet")
		return
	}
	for _, entry := range entries {
		if entry.Unusable != "" {
			// An entry whose timestamp could not be read has no age to
			// report: time.Since(zero) is a hundred thousand days, which
			// reads as a bug rather than as "this file is unreadable".
			if entry.CachedAt.IsZero() {
				add(LevelWarning, "cache."+entry.Key,
					"%s, will be refetched", entry.Unusable)
				continue
			}
			add(LevelWarning, "cache."+entry.Key,
				"%s, will be refetched (cached %s ago)", entry.Unusable,
				humanAge(time.Since(entry.CachedAt)))
			continue
		}
		add(LevelInfo, "cache."+entry.Key, "cached %s ago, valid for %s",
			humanAge(time.Since(entry.CachedAt)), ttl)
	}
}

// hoursPerDay is the divisor humanAge reports whole days with.
const hoursPerDay = 24

// humanAge renders a duration the way someone reads an age: whole days, then
// hours, then minutes. A cache entry's exact seconds are never the question.
func humanAge(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "less than a minute"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < hoursPerDay*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/hoursPerDay))
	}
}

// expandHome resolves a leading ~ so a check stats the path gits would use.
// An unexpandable value is returned unchanged and fails its own check with a
// message naming it, rather than being silently skipped.
func expandHome(path string) string {
	expanded, err := homedir.Expand(path)
	if err != nil {
		return path
	}
	return expanded
}

// render writes the report as Result Output. The findings are the result of
// this command — the thing to pipe into a grep or a ticket — so they go to
// Out, not Err, even though every one of them is about something being wrong.
func render(format string, report Report, deps types.RuntimeCLI) error {
	if format == bulk.FormatJSON {
		return jsonout.WriteValue(deps.Out, report)
	}

	for _, f := range report.Findings {
		style := deps.Theme.Normal
		switch f.Level {
		case LevelError:
			style = deps.Theme.Error
		case LevelWarning:
			style = deps.Theme.Warning
		case LevelInfo:
			style = deps.Theme.StatusDim
		}
		lipgloss.Fprintf(deps.Out, "%s %s %s\n",
			style.Render(fmt.Sprintf("%-7s", f.Level)),
			deps.Theme.RepoTitle.Render(f.Subject),
			f.Message,
		)
	}
	return nil
}
