package bulk

import (
	"fmt"

	"charm.land/lipgloss/v2"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cli"
	"github.com/rafi/gits/internal/types"
)

// Lines is the stock renderer: one line per repository — the padded display
// path followed by the body's text, or by its bare error — as Result Output,
// a blank line between projects, then the error epilogue as Diagnostic
// Output. Four of the five Bulk Commands hand their results straight to it.
func Lines(res Results[string], deps types.RuntimeCLI) error {
	width := 0
	for i, r := range res.Results {
		if i == 0 || r.Repo.ProjectKey != res.Results[i-1].Repo.ProjectKey {
			if i > 0 {
				fmt.Fprintln(deps.Out)
			}
			width = maxPathWidth(r.Repo.Project, deps.HomeDir)
		}
		title := cli.RepoTitle(r.Repo.Repository, r.Repo.Project, deps.HomeDir, deps.Theme).
			Width(width)
		body := r.Value
		if r.Err != nil {
			body = deps.Theme.Error.Render(r.Err.Error())
		}
		lipgloss.Fprintln(deps.Out, indentMultiline(fmt.Sprintf("%s %s", title, body)))
	}
	return Epilogue(res, deps)
}

// Epilogue writes the error epilogue as Diagnostic Output and returns the
// error that decides the run's exit code: nil when nothing counted, since
// warnings are listed on their repositories' lines and not here.
func Epilogue[T any](res Results[T], deps types.RuntimeCLI) error {
	return cli.RenderErrors(deps.Err, res.Errors(), true)
}

// maxPathWidth returns the rendered width of the widest repository display
// path in a project, so the lines beneath one project align.
func maxPathWidth(project domain.Project, homeDir string) int {
	width := 0
	for _, repo := range project.Repos {
		width = max(width, lipgloss.Width(cli.RepoRelPath(project, repo, homeDir)))
	}
	return width
}

// indentMultiline reformats a rendered repository line so its continuation
// lines (any after the first) are indented and prefixed with "> ". This keeps
// multi-line git output and errors visually attached to their row instead of
// bleeding out to the left margin. A single-line input is returned unchanged.
func indentMultiline(line string) string {
	return cli.IndentContinuation(line, "    > ")
}
