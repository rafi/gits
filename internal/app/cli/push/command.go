// Package push is the view side of `gits push`: it resolves what to run on,
// drives the engine, and renders each report as a line or as JSON.
package push

import (
	"context"

	"github.com/rafi/gits/internal/app"
	"github.com/rafi/gits/internal/app/cli/output"
	"github.com/rafi/gits/internal/app/cli/pick"
	"github.com/rafi/gits/internal/app/cli/progress"
	"github.com/rafi/gits/internal/git"
	"github.com/rafi/gits/internal/service"
	"github.com/rafi/gits/internal/service/push"
	"github.com/rafi/gits/internal/service/run"
)

// ExecPush pushes project repositories, or a specific repo, to their
// Upstream, rendering the results as lines or as the JSON envelope. See
// docs/adr/0002-push-safety-model.md for what it deliberately cannot do.
//
// Args: (optional)
//   - project name
//   - repo
func ExecPush(format string, opts git.PushOptions, args []string, deps app.RuntimeCLI) error {
	// Validate before anything is loaded or selected, so a rejected flag
	// combination or a typo'd format costs neither a provider round-trip nor
	// a single remote.
	if err := output.ValidateFormat(format); err != nil {
		return err
	}
	if err := opts.Validate(); err != nil {
		return err
	}

	// What the command runs on is settled — prompting included — before the
	// engine is handed anything, so nothing it does can fail over an argument.
	target, err := pick.Target(args, deps)
	if err != nil {
		return err
	}

	res := run.Command[push.Report]{
		Name: "push",
		Verb: "pushing",
		Do: func(ctx context.Context, repo run.Repo, rt service.Runtime) (push.Report, error) {
			return push.Repo(ctx, repo, opts, rt)
		},
		Progress: progress.New(deps.Err),
	}.Run(target, deps.Runtime)
	return output.Render(res, format, view, deps)
}
