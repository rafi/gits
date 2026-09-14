package exec

import (
	"github.com/rafi/gits/internal/app/cli/render/output"
	"github.com/rafi/gits/internal/app/cli/render/style"
	"github.com/rafi/gits/internal/service/exec"
)

// view is how an exec report reaches the two formats: styled for the table,
// plain for the document.
var view = output.View[exec.Report]{Line: Line, Text: Text}

// Line renders one report as the body of its repository's line.
func Line(r exec.Report, th style.Theme) string {
	return th.GitOutput.Render(r.Output)
}

// Text renders the same content for the JSON document, unstyled.
func Text(r exec.Report) string {
	return r.Output
}
