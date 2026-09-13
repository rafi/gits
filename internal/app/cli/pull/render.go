package pull

import (
	"fmt"

	"github.com/rafi/gits/internal/app/cli/output"
	"github.com/rafi/gits/internal/app/cli/style"
	"github.com/rafi/gits/internal/service/pull"
)

// view is how a pull report reaches the two formats: styled for the table,
// plain for the document.
var view = output.View[pull.Report]{Line: Line, Text: Text}

// Line renders one report as the body of its repository's line: the branch
// and the upstream it was pulled from, then git's own output in the theme's
// git-output style.
func Line(r pull.Report, th style.Theme) string {
	if r.Branch == "" {
		return ""
	}
	return fmt.Sprintf("[%s <- %s] %s", r.Branch, r.Upstream, th.GitOutput.Render(r.Output))
}

// Text renders the same content for the JSON document, unstyled.
func Text(r pull.Report) string {
	if r.Branch == "" {
		return ""
	}
	return fmt.Sprintf("[%s <- %s] %s", r.Branch, r.Upstream, r.Output)
}
