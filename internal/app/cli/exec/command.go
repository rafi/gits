// Package exec is the view side of `gits exec`: it resolves what to run on,
// drives the engine, and renders each report as a line or as JSON.
package exec

import (
	"context"

	"github.com/rafi/gits/internal/app"
	"github.com/rafi/gits/internal/app/cli/output"
	"github.com/rafi/gits/internal/app/cli/pick"
	"github.com/rafi/gits/internal/app/cli/progress"
	"github.com/rafi/gits/internal/service"
	"github.com/rafi/gits/internal/service/exec"
	"github.com/rafi/gits/internal/service/run"
)

// ErrNoCommand is returned when no command follows the `--` separator. It is
// the service's error, re-exported so the cobra wiring keeps naming one
// package.
var ErrNoCommand = exec.ErrNoCommand

// Exec runs command in every repository of a project, or in a specific repo,
// rendering the results as lines or as the JSON envelope. It is exec.Exec
// rather than the ExecExec the other commands' naming would give, because
// the package is already named for the verb.
//
// Args: (optional)
//   - project name
//   - repo or sub-project name
//
// command is the argv to run, already split from args at the `--` separator.
func Exec(format string, command []string, args []string, deps app.RuntimeCLI) error {
	// Both checks precede the load, so neither mistake costs a provider
	// round-trip or an interactive prompt.
	if err := output.ValidateFormat(format); err != nil {
		return err
	}
	if len(command) == 0 {
		return ErrNoCommand
	}

	// What the command runs on is settled — prompting included — before the
	// engine is handed anything, so nothing it does can fail over an argument.
	target, err := pick.Target(args, deps)
	if err != nil {
		return err
	}

	res := run.Command[exec.Report]{
		Name: "exec",
		Verb: "running",
		Do: func(ctx context.Context, repo run.Repo, _ service.Runtime) (exec.Report, error) {
			return exec.Repo(ctx, command, repo)
		},
		Progress: progress.New(deps.Err),
	}.Run(target, deps.Runtime)
	return output.Render(res, format, view, deps)
}
