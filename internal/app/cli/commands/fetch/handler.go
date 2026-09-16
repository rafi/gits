// Package fetch is the view side of `gits fetch`: it resolves what to run
// on, drives the engine, and renders each report as a line or as JSON.
package fetch

import (
	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/app"
	"github.com/rafi/gits/internal/app/cli/interaction/progress"
	pick "github.com/rafi/gits/internal/app/cli/interaction/select"
	"github.com/rafi/gits/internal/app/cli/render/output"
	"github.com/rafi/gits/internal/runtime/command"
	"github.com/rafi/gits/internal/service/fetch"
)

// ExecFetch runs fetch on project repositories, or on a specific repo,
// rendering the results as lines or as the JSON envelope.
//
// Args: (optional)
//   - project name
//   - repo or sub-project name
func ExecFetch(format string, tags domain.TagSet, args []string, deps app.RuntimeCLI) error {
	// Validate before anything is loaded or selected, so a typo'd format never
	// costs a provider round-trip or an interactive prompt.
	if err := output.ValidateFormat(format); err != nil {
		return err
	}

	// What the command runs on is settled — prompting included — before the
	// engine is handed anything, so nothing it does can fail over an argument.
	target, err := pick.TargetWithTags(args, tags, deps)
	if err != nil {
		return err
	}

	res := command.Command[fetch.Report]{
		Name:     "fetch",
		Verb:     "fetching",
		Do:       fetch.Repo,
		Progress: progress.New(deps.Err),
	}.Run(target, deps.Runtime)
	return output.Render(res, format, view, deps)
}
