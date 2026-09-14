// Package sync is the view side of `gits sync`: it refreshes each Project's
// cached repository list and narrates the run, so a command that spends a
// minute talking to a code forge says what it is doing while it does it.
package sync

import (
	"fmt"

	"charm.land/lipgloss/v2"

	"github.com/rafi/gits/internal/app"
	"github.com/rafi/gits/internal/app/cli/render/style"
	"github.com/rafi/gits/internal/format"
	"github.com/rafi/gits/internal/service/sync"
)

// ExecSync refreshes the cache for the given projects.
//
// Args: (optional)
//   - project names
func ExecSync(args []string, deps app.RuntimeCLI) error {
	names, err := sync.Targets(args, deps.Runtime)
	if err != nil {
		return err
	}

	total := 0
	for i, name := range names {
		// The Project is named before it is contacted, so whatever the forge
		// is being slow about is on screen while it is slow, and a run that
		// fails has already said which Project it failed on.
		lipgloss.Fprintf(deps.Out, "%s %s ",
			deps.Theme.StatusDim.Render(fmt.Sprintf("[%d/%d]", i+1, len(names))),
			deps.Theme.ProjectTitle.Render(name))

		result, err := sync.One(name, deps.Runtime)
		if err != nil {
			lipgloss.Fprintln(deps.Out, deps.Theme.Error.Render("failed"))
			return err
		}
		lipgloss.Fprintln(deps.Out, renderResult(result, deps.HomeDir, deps.Theme))
		total += result.Repos
	}
	lipgloss.Fprintln(deps.Err, renderSummary(len(names), total, deps.Theme))
	return nil
}

// renderResult completes a Project's line with what refreshing it found:
// where its repositories came from, whether a cache was dropped, where they
// live locally, and how many came back.
//
//	[github:acme] flushed · ~/code/acme · 42 repositories
//
// The path is what a user does something with next; the code forge's own ID
// for the account is not, so it is left to `-o json` consumers.
func renderResult(result sync.Result, homeDir string, theme style.Theme) string {
	line := theme.Provider.Render(sourceLabel(result))
	if result.Flushed {
		line += theme.StatusDim.Render(" flushed")
	}
	if result.Path != "" {
		line += theme.StatusDim.Render(" · ") +
			theme.RepoPath.Render(format.Path(result.Path, homeDir))
	}
	return line + theme.StatusDim.Render(" · ") + theme.StatusAdded.Render(
		fmt.Sprintf("%d %s", result.Repos, plural(result.Repos, "repository", "repositories")))
}

// renderSummary closes the run on Diagnostic Output: it is about the run
// rather than part of its result, so a script reading the lines above is
// unaffected by it.
func renderSummary(projects, repos int, theme style.Theme) string {
	return theme.StatusFooter.Render(fmt.Sprintf("Synchronized %d %s, %d %s.",
		projects, plural(projects, "project", "projects"),
		repos, plural(repos, "repository", "repositories")))
}

// sourceLabel names the code forge a Project's repositories came from, as
// `[type:search]`. Only remote-backed Projects are synced, so there is
// always a type; a source with no search term would not have validated.
func sourceLabel(result sync.Result) string {
	if result.Source.Search == "" {
		return "[" + result.Source.Type + "]"
	}
	return "[" + result.Source.Type + ":" + result.Source.Search + "]"
}

// plural picks the form matching n.
func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
