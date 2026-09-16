// Package push is the view side of `gits push`: it resolves what to run on,
// drives the engine, and renders each report as a line or as JSON.
package push

import (
	"context"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/app"
	"github.com/rafi/gits/internal/app/cli/interaction/progress"
	pick "github.com/rafi/gits/internal/app/cli/interaction/select"
	"github.com/rafi/gits/internal/app/cli/render/output"
	"github.com/rafi/gits/internal/infra/git"
	coreruntime "github.com/rafi/gits/internal/runtime"
	"github.com/rafi/gits/internal/runtime/command"
	"github.com/rafi/gits/internal/service/push"
)

// ExecPush pushes project repositories, or a specific repo, to their
// Upstream, rendering the results as lines or as the JSON envelope. Bulk push
// is safe by construction: it cannot force, mirror, or create upstreams.
//
// Args: (optional)
//   - project name
//   - repo
func ExecPush(
	format string, opts git.PushOptions, tags domain.TagSet,
	args []string, deps app.RuntimeCLI,
) error {
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
	target, err := pick.TargetWithTags(args, tags, deps)
	if err != nil {
		return err
	}

	res := command.Command[push.Report]{
		Name: "push",
		Verb: "pushing",
		Do: func(ctx context.Context, repo command.Repo, rt coreruntime.Runtime) (push.Report, error) {
			return push.Repo(ctx, repo, opts, rt)
		},
		Progress: progress.New(deps.Err),
	}.Run(target, deps.Runtime)
	return output.Render(res, format, view, deps)
}
