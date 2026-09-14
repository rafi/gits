// Package app holds the view-facing dependencies of a client. A command body
// receives business dependencies only; anything that renders receives a
// Presenter, so reaching for a theme from a body is a compile error.
package app

import (
	"io"

	"github.com/rafi/gits/internal/app/cli/render/style"
	coreruntime "github.com/rafi/gits/internal/runtime"
)

// Presenter is what a view needs to render: where output goes, and how it
// looks.
type Presenter struct {
	// Out is the Result Output destination: what the command was asked for —
	// the status table, the JSON document, the list of names — and nothing
	// else, so any command's output can be piped or redirected unfiltered.
	Out io.Writer
	// Err is the Diagnostic Output destination: everything a command emits
	// about producing its Result Output — live per-repository progress, the
	// summary footer, the error epilogue.
	//
	// The live progress reporter is built from this writer: progress.New
	// sniffs it for a terminal, so a test buffer yields the engine's no-op
	// and no ANSI reaches the assertions.
	Err io.Writer

	Theme style.Theme
}

// RuntimeCLI is the full set of dependencies a CLI command runs with: the
// business runtime every client shares, plus this client's view.
type RuntimeCLI struct {
	coreruntime.Runtime
	Presenter
}
