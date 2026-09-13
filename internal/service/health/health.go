// Package health answers the questions no ordinary command is in a position
// to raise: whether git is on PATH, whether a project's `path:` exists,
// whether a cache entry will ever be read again.
//
// A command loads the projects it was asked about and says what it found; it
// has no reason to notice that some *other* project's `path:` does not exist.
// Check asks all of it at once, so "why is gits behaving strangely" has one
// answer rather than a hunt.
//
// It never contacts a Provider Source. A diagnostic that waits on the network
// or prompts for a passphrase is one people stop running, and every check
// here is answerable from the config file, the filesystem and PATH.
package health

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/mitchellh/go-homedir"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/infra/cache"
	"github.com/rafi/gits/internal/service"
)

// Check runs every diagnostic and returns the findings in the order they were
// made, which groups them by the thing inspected.
func Check(rt service.Runtime) []Finding {
	var findings []Finding
	add := func(level Level, scope Scope, subject, format string, args ...any) {
		findings = append(findings, Finding{
			Level:   level,
			Scope:   scope,
			Subject: subject,
			Message: fmt.Sprintf(format, args...),
		})
	}

	checkConfig(rt, add)
	checkProjects(rt, add)
	checkBinaries(rt, add)
	checkCache(rt, add)
	return findings
}

// addFunc records one finding. Passed to each check so they compose without
// each returning and merging its own slice.
type addFunc func(level Level, scope Scope, subject, format string, args ...any)

// checkConfig reports the config file in use, and the warnings gathered while
// loading it — unknown keys among them. Those are already surfaced on every
// run; repeating them here is deliberate, since this is what someone asks
// when they want everything at once.
func checkConfig(rt service.Runtime, add addFunc) {
	if rt.ConfigPath == "" {
		add(LevelWarning, ScopeConfig, "config",
			"no config file found; gits is running with an empty configuration")
		return
	}
	add(LevelInfo, ScopeConfig, "config", "using %s", rt.ConfigPath)

	for _, w := range rt.ConfigWarnings {
		add(LevelError, ScopeConfig, "config", "%s", w)
	}
}

// checkProjects inspects each configured project without loading any of them:
// a Provider Source is never contacted, so this stays offline and prompt-free.
func checkProjects(rt service.Runtime, add addFunc) {
	if len(rt.Projects) == 0 {
		add(LevelWarning, ScopeConfig, "projects", "no projects are configured")
		return
	}

	for _, name := range rt.Projects.SortedNames() {
		project := rt.Projects[name]
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
		add(LevelError, ScopeConfig, subject,
			"no `path:` on the project and no `dir:` on the repository, so there is nowhere for it to live")
	case repo.Dir != "" && project.Path == "" && !filepath.IsAbs(expandHome(repo.Dir)):
		// A relative `dir:` is resolved against the Project Path, and without
		// one it would silently resolve against the working directory.
		add(LevelError, ScopeConfig, subject,
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
		add(LevelWarning, ScopeConfig, subject,
			"`path: %s` does not exist yet; `gits clone` will create it", project.Path)
	case err != nil:
		add(LevelError, ScopeConfig, subject, "`path: %s` cannot be read: %v", project.Path, err)
	case !info.IsDir():
		add(LevelError, ScopeConfig, subject, "`path: %s` is not a directory", project.Path)
	}
}

// versionProbeTimeout bounds asking a binary for its version. Generous for a
// local process, short enough that a wedged one cannot hang the report.
const versionProbeTimeout = 5 * time.Second

// checkBinaries reports the external programs gits shells out to. git is
// required by every command that touches a repository; the finder is needed
// only to pick something interactively, so its absence is a warning.
func checkBinaries(rt service.Runtime, add addFunc) {
	if path, err := exec.LookPath("git"); err != nil {
		add(LevelError, ScopeEnvironment, "git",
			"not found on PATH; every repository operation will fail")
	} else {
		add(LevelInfo, ScopeEnvironment, "git", "%s (%s)",
			binaryVersion(rt.Ctx, "git", "--version"), path)
	}

	finder := rt.Settings.Finder.Binary
	subject := "finder"
	if finder == "" {
		finder = "fzf"
	} else {
		subject = "finder (settings.finder.binary)"
	}
	if path, err := exec.LookPath(finder); err != nil {
		add(LevelWarning, ScopeEnvironment, subject,
			"%q not found on PATH; interactive selection is unavailable", finder)
	} else {
		add(LevelInfo, ScopeEnvironment, subject, "%s (%s)",
			binaryVersion(rt.Ctx, finder, "--version"), path)
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
func checkCache(rt service.Runtime, add addFunc) {
	if rt.Settings.Cache != nil && !*rt.Settings.Cache {
		add(LevelInfo, ScopeEnvironment, "cache", "disabled by `settings.cache: false`")
		return
	}

	dir, err := cache.Dir()
	if err != nil {
		add(LevelError, ScopeEnvironment, "cache", "cannot locate the cache directory: %v", err)
		return
	}
	add(LevelInfo, ScopeEnvironment, "cache", "directory %s", dir)

	// An unparseable cacheTTL falls back to its default and is already
	// warned about at startup, so the fallback is used here without a second
	// complaint.
	ttl, _ := rt.Settings.CacheTTLDuration()
	entries, err := cache.Entries(ttl)
	if err != nil {
		add(LevelError, ScopeEnvironment, "cache", "cannot read the cache directory: %v", err)
		return
	}
	if len(entries) == 0 {
		add(LevelInfo, ScopeEnvironment, "cache", "no projects are cached yet")
		return
	}
	for _, entry := range entries {
		if entry.Unusable != "" {
			// An entry whose timestamp could not be read has no age to
			// report: time.Since(zero) is a hundred thousand days, which
			// reads as a bug rather than as "this file is unreadable".
			if entry.CachedAt.IsZero() {
				add(LevelWarning, ScopeEnvironment, "cache."+entry.Key,
					"%s, will be refetched", entry.Unusable)
				continue
			}
			add(LevelWarning, ScopeEnvironment, "cache."+entry.Key,
				"%s, will be refetched (cached %s ago)", entry.Unusable,
				humanAge(time.Since(entry.CachedAt)))
			continue
		}
		add(LevelInfo, ScopeEnvironment, "cache."+entry.Key, "cached %s ago, valid for %s",
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
