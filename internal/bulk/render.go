package bulk

import (
	"fmt"
	"io"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cli"
	"github.com/rafi/gits/internal/cli/jsonout"
	"github.com/rafi/gits/internal/types"
)

// The output formats a line Bulk Command renders: the stock lines, or the
// JSON envelope.
const (
	FormatTable = "table"
	FormatJSON  = "json"
)

// Formats are the accepted values of `-o`, in the order they are offered and
// listed. ValidateFormat accepts exactly these, so the flag's help text and
// its shell completion are built from this slice rather than restating it.
func Formats() []string {
	return []string{FormatTable, FormatJSON}
}

// ValidateFormat rejects everything but table and json. `list`'s other styles
// (name, tree, wide) are shapes a Bulk Command has no meaning for, and
// quietly falling back to the table would answer a question the user did not
// ask. A command calls it before loading anything, so a typo'd format never
// costs a provider round-trip or an interactive prompt.
//
// `gits doctor` renders the same pair and shares this validator, though it is
// not a Bulk Command: the vocabulary is the flag's, not the traversal's.
func ValidateFormat(format string) error {
	switch format {
	case FormatTable, FormatJSON:
		return nil
	default:
		return fmt.Errorf("unknown output format %q, want %s",
			format, strings.Join(Formats(), " or "))
	}
}

// Render writes the results in the given format — already validated — and
// returns the error that decides the run's exit code, which differs by
// format: see Lines and JSON.
func Render(res Results[string], format string, deps types.RuntimeCLI) error {
	if format == FormatJSON {
		return JSON(res, deps)
	}
	return Lines(res, deps)
}

// Lines is the stock renderer: one line per repository — the padded display
// path followed by the body's text, or by its bare error — as Result Output,
// a blank line between projects, then the error epilogue as Diagnostic
// Output. Every Bulk Command but status renders its table form through it.
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

// JSON is the machine-readable renderer: the envelope `list -o json` and
// `status -o json` share, with what the command made of each repository
// nested under the command's own name — `"pull": {"output": "…"}`. The
// document's tree is the one the run visited, or empty when the named
// project was skipped.
//
// The exit-code rule is `status -o json`'s: a repository's outcome — its
// output, its pass-over, its failure — is data in the document, so none of
// them fails the run and no error epilogue is printed. An interrupted run
// still fails, because the document is incomplete and nothing inside it
// says so.
func JSON(res Results[string], deps types.RuntimeCLI) error {
	if err := writeJSON(deps.Out, res); err != nil {
		return err
	}
	return res.Interrupted
}

// writeJSON builds the document from the tree and the results and writes it.
func writeJSON(w io.Writer, res Results[string]) error {
	env := jsonout.Envelope{}
	if res.Project.Name == "" {
		// The named project was skipped: nothing ran, nothing to document.
		return jsonout.Write(w, env)
	}
	index := make(map[string]Result[string], len(res.Results))
	for _, r := range res.Results {
		index[r.Repo.Key()] = r
	}
	env[res.Project.Name] = buildNode(res.Project, res.Command, index)
	return jsonout.Write(w, env)
}

// buildNode converts one project subtree, looking each repository's result
// up by identity so the tree, not the order results arrived in, shapes the
// document. A repository the run never started (canceled before it was
// dequeued) is present with its identity and state and no outcome, exactly
// as a repository the guard turned back is: the command did not run for
// either.
func buildNode(p domain.Project, command string, index map[string]Result[string]) jsonout.Project {
	node := jsonout.NewProject(p)
	for _, repo := range p.Repos {
		out := jsonout.NewRepository(repo)
		if r, ok := index[repo.Key()]; ok && !r.Guarded {
			out.Command = command
			out.Outcome = outcome(r)
		}
		node.Repos = append(node.Repos, out)
	}
	for _, sub := range p.SubProjects {
		node.SubProjects = append(node.SubProjects, buildNode(sub, command, index))
	}
	return node
}

// outcome converts one result the body produced. The body's text is what the
// table shows, terminal styling and padding included; the document carries
// the text alone. A warning is the documented pass-over the table shows on
// the line without failing the run, and it is reported as such rather than
// as an error, so a consumer can tell "nothing to do here" from "this
// failed".
func outcome(r Result[string]) *jsonout.Outcome {
	switch {
	case r.Err == nil:
		return &jsonout.Outcome{Output: plain(r.Value)}
	case types.IsWarning(r.Err):
		return &jsonout.Outcome{Skipped: plain(r.Err.Error())}
	default:
		return &jsonout.Outcome{Error: plain(r.Err.Error())}
	}
}

// plain strips what a body adds for the terminal: styling, and the whitespace
// a line's layout leaves at its ends.
func plain(s string) string {
	return strings.TrimSpace(ansi.Strip(s))
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
