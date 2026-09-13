package style

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/service/run"
)

// AbortOnRepoState writes an error message as Diagnostic Output and aborts if
// the repository is in an error state. Used by single-repo/interactive
// callers; a Bulk Command declares the states it accepts instead, and the
// line renderer shows the guard failure on the repository's own line.
//
// The message is terminated here, so a caller that writes a repository title
// ahead of it shares that line and appends no newline of its own.
func AbortOnRepoState(w io.Writer, repo domain.Repository, style lipgloss.Style) error {
	err := run.StateError(repo)
	lipgloss.Fprintln(w, style.Render(err.Error()))
	return run.RepoError(err, repo)
}

// IndentContinuation keeps the first line of s untouched and prefixes every
// continuation line with prefix. A single-line input is returned unchanged.
// The error epilogue and a run's result lines each pass their own
// prefix; the pass over the string is the same one.
func IndentContinuation(s, prefix string) string {
	head, rest, found := strings.Cut(s, "\n")
	if !found {
		return s
	}
	lines := strings.Split(rest, "\n")
	for i, l := range lines {
		lines[i] = prefix + l
	}
	return head + "\n" + strings.Join(lines, "\n")
}

// RenderErrors writes the run's error epilogue as Diagnostic Output and
// returns the error that decides the exit code — nil when nothing counted.
func RenderErrors(w io.Writer, errs []error, excludeWarnings bool) error {
	out := []string{}
	count := 0
	for _, err := range errs {
		if excludeWarnings && domain.IsWarning(err) {
			continue
		}
		count++
		out = append(out, IndentContinuation(fmt.Sprintf("  - %s", err), "      > "))
	}
	if count < 1 {
		return nil
	}
	title := "error" + Plural(count)
	out = append([]string{"", fmt.Sprintf("%d %s:", count, title), ""}, out...)
	out = append(out, "")
	fmt.Fprintln(w, strings.Join(out, "\n"))
	return errors.New("completed with errors")
}

// Plural returns the "s" suffix for a count-aware label.
func Plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
