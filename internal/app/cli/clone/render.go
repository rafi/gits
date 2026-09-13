package clone

import (
	"github.com/rafi/gits/internal/app/cli/output"
	"github.com/rafi/gits/internal/app/cli/style"
	"github.com/rafi/gits/internal/service/clone"
)

// view is how a clone report reaches the two formats: styled for the table,
// plain for the document.
var view = output.View[clone.Report]{Line: Line, Text: Text}

// Line renders one report as the body of its repository's line.
func Line(r clone.Report, th style.Theme) string {
	return th.GitOutput.Render(r.Output)
}

// Text renders the same content for the JSON document, unstyled.
func Text(r clone.Report) string {
	return r.Output
}
