package fetch

import (
	"fmt"

	"github.com/rafi/gits/internal/app/cli/output"
	"github.com/rafi/gits/internal/app/cli/style"
	"github.com/rafi/gits/internal/service/fetch"
)

// view is how a fetch report reaches the two formats: styled for the table,
// plain for the document.
var view = output.View[fetch.Report]{Line: Line, Text: Text}

// Line renders one report as the body of its repository's line.
func Line(r fetch.Report, th style.Theme) string {
	return body(r, th.GitOutput.Render(r.Output))
}

// Text renders the same content for the JSON document, unstyled.
func Text(r fetch.Report) string {
	return body(r, r.Output)
}

// body prefixes git's output with the repository's real location, when its
// line's title does not already say it.
func body(r fetch.Report, out string) string {
	if r.RepoPath == "" {
		return out
	}
	return fmt.Sprintf("%s %s", r.RepoPath, out)
}
