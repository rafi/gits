package push

import (
	"fmt"

	"github.com/rafi/gits/internal/app/cli/render/output"
	"github.com/rafi/gits/internal/app/cli/render/style"
	"github.com/rafi/gits/internal/service/push"
)

// view is how a push report reaches the two formats: styled for the table,
// plain for the document.
var view = output.View[push.Report]{Line: Line, Text: Text}

// Line renders one report as the body of its repository's line.
func Line(r push.Report, th style.Theme) string {
	return body(r, th.GitOutput.Render(r.Output))
}

// Text renders the same content for the JSON document, unstyled.
func Text(r push.Report) string {
	return body(r, r.Output)
}

// body names the branch and where it went, unless git chose the destination
// itself and there is no single branch to name.
func body(r push.Report, out string) string {
	if r.Branch == "" {
		return out
	}
	return fmt.Sprintf("[%s -> %s] %s", r.Branch, r.Upstream, out)
}
