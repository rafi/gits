package bulk

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/rafi/gits/internal/cli"
	"github.com/rafi/gits/internal/types"
)

// Render draws res's groups as Result Output and returns every result's error
// in stable tree order, the interruption included. A renderer that is not
// Lines calls this and then Epilogue, so the two share one traversal.
//
// It encodes that traversal's contract once, and each clause is a mistake a
// hand-rolled loop makes silently: a nil result slot is skipped (the
// repository was never started), errors are collected from every result —
// including the ones a body chooses not to show — a blank line separates
// printed groups, and each shown group gets its project title above its body,
// unless a single repository was named. The body callback returns the group's
// rendered content ("" prints nothing under the title) and whether the group
// appears at all.
func Render[T any](
	res Results[T],
	deps types.RuntimeCLI,
	body func(Group[T]) (string, bool),
) []error {
	var errs []error
	printed := 0
	for _, g := range res.Groups {
		for _, r := range g.Results {
			if r == nil {
				continue // not started (canceled before dequeue)
			}
			if r.Err != nil {
				errs = append(errs, r.Err)
			}
		}

		content, show := body(g)
		if !show {
			continue
		}
		if printed > 0 {
			fmt.Fprintln(deps.Out)
		}
		if !res.Single {
			lipgloss.Fprintln(deps.Out, cli.ProjectTitleWithBullet(g.Project, deps.Theme))
		}
		printed++
		if content != "" {
			lipgloss.Fprintln(deps.Out, content)
		}
	}
	if res.Interrupted != nil {
		errs = append(errs, res.Interrupted)
	}
	return errs
}

// Epilogue decides the run's outcome from the errors Render collected. A whole
// tree writes the error epilogue as Diagnostic Output and fails when anything
// counted; a single named repository returns its own error as itself, so a
// warning still downgrades the exit code at the root instead of being counted
// as a failure.
func Epilogue[T any](res Results[T], errs []error, deps types.RuntimeCLI) error {
	if res.Single {
		if len(errs) > 0 {
			return errs[0]
		}
		return nil
	}
	return cli.RenderErrors(deps.Err, errs, true)
}

// Lines is the stock renderer: one line per repository — the padded title
// followed by the body's text, or by its error — drawn under each project's
// title as Result Output, then the error epilogue as Diagnostic Output. Four
// of the five Bulk Commands set it as a field value.
func Lines(res Results[string], deps types.RuntimeCLI) error {
	errs := Render(res, deps, func(g Group[string]) (string, bool) {
		var lines []string
		for _, r := range g.Results {
			if r == nil {
				continue
			}
			lines = append(lines, indentMultiline(repoLineOf(r, deps).String()))
		}
		return strings.Join(lines, "\n"), true
	})
	return Epilogue(res, errs, deps)
}

// repoLine is one result line: the padded repository title followed by either
// the styled error or the command-specific body.
type repoLine struct {
	Title      lipgloss.Style
	Body       string
	Err        error
	ErrorStyle lipgloss.Style
}

func (r repoLine) String() string {
	if r.Err != nil {
		return fmt.Sprintf("%s %s", r.Title, r.ErrorStyle.Render(r.Err.Error()))
	}
	return fmt.Sprintf("%s %s", r.Title, r.Body)
}

// repoLineOf assembles one result's line from the title the module measured
// and the body's own text.
func repoLineOf(res *Result[string], deps types.RuntimeCLI) repoLine {
	return repoLine{
		Title:      res.Repo.Title,
		Body:       res.Value,
		Err:        res.Err,
		ErrorStyle: deps.Theme.Error,
	}
}

// indentMultiline reformats a rendered repository line so its continuation
// lines (any after the first) are indented and prefixed with "> ". This keeps
// multi-line git output and errors visually attached to their row instead of
// bleeding out to the left margin. A single-line input is returned unchanged.
func indentMultiline(line string) string {
	return cli.IndentContinuation(line, "    > ")
}
