// Package pull implements `gits pull`, the Bulk Command that fast-forwards
// every Repository in a Project.
package pull

import (
	"context"
	"fmt"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/app"
	"github.com/rafi/gits/internal/app/cli/output"
	"github.com/rafi/gits/internal/app/cli/pick"
	"github.com/rafi/gits/internal/app/cli/progress"
	"github.com/rafi/gits/internal/git"
	"github.com/rafi/gits/internal/service"
	"github.com/rafi/gits/internal/service/run"
)

// ExecPull runs pull --ff-only on project repositories, or on a specific
// repo, rendering the results as lines or as the JSON envelope.
//
// Args: (optional)
//   - project name
//   - repo
func ExecPull(format string, args []string, deps app.RuntimeCLI) error {
	// Validate before anything is loaded or selected, so a typo'd format never
	// costs a provider round-trip or an interactive prompt.
	if err := output.ValidateFormat(format); err != nil {
		return err
	}

	// What the command runs on is settled — prompting included — before the
	// engine is handed anything, so nothing it does can fail over an argument.
	target, err := pick.Target(args, deps)
	if err != nil {
		return err
	}

	res := run.Command[string]{
		Name:     "pull",
		Verb:     "pulling",
		Do:       pullRepo(deps),
		Progress: progress.New(deps.Err),
	}.Run(target, deps.Runtime)
	return output.Render(res, format, deps)
}

// pullRepo returns a body that pulls one repository into its result line.
func pullRepo(deps app.RuntimeCLI) func(
	context.Context, run.Repo, service.Runtime,
) (string, error) {
	return func(ctx context.Context, repo run.Repo, rt service.Runtime) (string, error) {
		head, err := rt.Git.HeadUpstream(ctx, repo.AbsPath)
		if err != nil {
			return "", err
		}

		switch {
		case head.Upstream == "":
			// There is nowhere to pull from
			return "", domain.NewWarning("skipped: %s", git.ErrNoUpstream)
		case head.Gone:
			// Branch is gone, merged and deleted?
			return "", domain.NewWarning("skipped: %s: %s", head.Upstream, git.ErrUpstreamGone)
		}

		out, err := rt.Git.Pull(ctx, repo.AbsPath)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf(
			"[%s <- %s] %s",
			head.Branch,
			head.Upstream,
			deps.Theme.GitOutput.Render(out),
		), nil
	}
}
