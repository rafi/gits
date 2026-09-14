// Package output renders a run's results for the terminal: the stock lines,
// the JSON envelope, and the `-o` vocabulary that chooses between them.
package output

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/app"
	"github.com/rafi/gits/internal/app/cli/render/style"
	"github.com/rafi/gits/internal/format"
	"github.com/rafi/gits/internal/runtime/command"
	"github.com/rafi/gits/internal/runtime/json"
)

// The output formats a line command renders: the stock lines, or the JSON
// envelope.
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
// (name, tree, wide) are shapes a bulk run has no meaning for, and quietly
// falling back to the table would answer a question the user did not ask. A
// command calls it before loading anything, so a typo'd format never costs a
// provider round-trip or an interactive prompt.
//
// `gits doctor` renders the same pair and shares this validator, though it
// runs nothing across repositories: the vocabulary is the flag's, not the
// traversal's.
func ValidateFormat(format string) error {
	switch format {
	case FormatTable, FormatJSON:
		return nil
	default:
		return fmt.Errorf("unknown output format %q, want %s",
			format, strings.Join(Formats(), " or "))
	}
}

// View is how a command turns one repository's report into text. Line is the
// terminal form — styled with the theme, as the table shows it — and Text is
// the same content plain, for the JSON document. Two functions rather than
// one stripped string: the document is built from the report's own fields,
// so no renderer has to undo what another one added.
type View[T any] struct {
	Line func(T, style.Theme) string
	Text func(T) string
}

// Render writes the results in the given format — already validated — and
// returns the error that decides the run's exit code, which differs by
// format: see Lines and JSON.
func Render[T any](res command.Results[T], format string, v View[T], deps app.RuntimeCLI) error {
	if format == FormatJSON {
		return JSON(res, v, deps)
	}
	return Lines(res, v, deps)
}

// Skips reports the projects the run's configuration dropped as Diagnostic
// Output, so a project that vanished is distinguishable from an empty one.
// Every format prints them: they are about the run, not part of its result.
func Skips[T any](res command.Results[T], deps app.RuntimeCLI) {
	for _, name := range res.Skipped {
		fmt.Fprintf(deps.Err, "Skipping %s: excluded by configuration\n", name)
	}
}

// Lines is the stock renderer: one line per repository — the padded display
// path followed by the body's text, or by its bare error — as Result Output,
// a blank line between projects, then the error epilogue as Diagnostic
// Output. Every line command but status renders its table form through it.
func Lines[T any](res command.Results[T], v View[T], deps app.RuntimeCLI) error {
	Skips(res, deps)
	width := 0
	for i, r := range res.Results {
		if i == 0 || r.Repo.ProjectKey != res.Results[i-1].Repo.ProjectKey {
			if i > 0 {
				fmt.Fprintln(deps.Out)
			}
			width = maxPathWidth(r.Repo.Project, deps.HomeDir)
		}
		title := style.RepoTitle(r.Repo.Repository, r.Repo.Project, deps.HomeDir, deps.Theme).
			Width(width)
		body := v.Line(r.Value, deps.Theme)
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
func JSON[T any](res command.Results[T], v View[T], deps app.RuntimeCLI) error {
	Skips(res, deps)
	if err := writeJSON(deps.Out, res, v); err != nil {
		return err
	}
	return res.Interrupted
}

// writeJSON builds the document from the tree and the results and writes it.
func writeJSON[T any](w io.Writer, res command.Results[T], v View[T]) error {
	env := json.Envelope{}
	if res.Project.Name == "" {
		// The named project was skipped: nothing ran, nothing to document.
		return json.Write(w, env)
	}
	index := make(map[string]command.Result[T], len(res.Results))
	for _, r := range res.Results {
		index[r.Repo.Key()] = r
	}
	env[res.Project.Name] = buildNode(res.Project, res.Command, index, v)
	return json.Write(w, env)
}

// buildNode converts one project subtree, looking each repository's result
// up by identity so the tree, not the order results arrived in, shapes the
// document. A repository the run never started (canceled before it was
// dequeued) is present with its identity and state and no outcome, exactly
// as a repository the guard turned back is: the command did not run for
// either.
func buildNode[T any](
	p domain.Project, command string, index map[string]command.Result[T], v View[T],
) json.Project {
	node := json.NewProject(p)
	for _, repo := range p.Repos {
		out := json.NewRepository(repo)
		if r, ok := index[repo.Key()]; ok && !r.Guarded {
			out.Command = outcome(command, r, v)
		}
		node.Repos = append(node.Repos, out)
	}
	for _, sub := range p.SubProjects {
		node.SubProjects = append(node.SubProjects, buildNode(sub, command, index, v))
	}
	return node
}

// outcome converts one result the body produced, through the view's plain
// form. A warning is the documented pass-over, reported as such rather than
// as an error, so a consumer can tell "nothing to do here" from "this
// failed".
func outcome[T any](command string, r command.Result[T], v View[T]) *json.Command {
	switch {
	case r.Err == nil:
		return json.OK(command, strings.TrimSpace(v.Text(r.Value)))
	case domain.IsWarning(r.Err):
		return json.Skipped(command, strings.TrimSpace(r.Err.Error()))
	default:
		return json.Failed(command, errors.New(strings.TrimSpace(r.Err.Error())))
	}
}

// Epilogue writes the error epilogue as Diagnostic Output and returns the
// error that decides the run's exit code: nil when nothing counted, since
// warnings are listed on their repositories' lines and not here.
func Epilogue[T any](res command.Results[T], deps app.RuntimeCLI) error {
	return style.RenderErrors(deps.Err, Errors(res), true)
}

// Errors returns every result's error in tree order — a plain error wrapped
// with its repository's name and path, a warning as it is — followed by the
// interruption, if any. This is the list the error epilogue renders.
func Errors[T any](res command.Results[T]) []error {
	var errs []error
	for _, r := range res.Results {
		if r.Err == nil {
			continue
		}
		if _, ok := errors.AsType[*domain.Warning](r.Err); ok {
			errs = append(errs, r.Err)
			continue
		}
		errs = append(errs, command.RepoError(r.Err, r.Repo.Repository))
	}
	if res.Interrupted != nil {
		errs = append(errs, res.Interrupted)
	}
	return errs
}

// maxPathWidth returns the rendered width of the widest repository display
// path in a project, so the lines beneath one project align.
func maxPathWidth(project domain.Project, homeDir string) int {
	width := 0
	for _, repo := range project.Repos {
		width = max(width, lipgloss.Width(format.RepoRelPath(project, repo, homeDir)))
	}
	return width
}

// indentMultiline reformats a rendered repository line so its continuation
// lines (any after the first) are indented and prefixed with "> ". This keeps
// multi-line git output and errors visually attached to their row instead of
// bleeding out to the left margin. A single-line input is returned unchanged.
func indentMultiline(line string) string {
	return style.IndentContinuation(line, "    > ")
}
