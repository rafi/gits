// Package push implements `gits push`, the Bulk Command that pushes each
// Repository's current branch to its Upstream.
// See: docs/adr/0002-push-safety-model.md.
package push

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

	res := run.Command[string]{
		Name:     "push",
		Verb:     "pushing",
		Do:       pushRepo(opts, deps),
		Progress: progress.New(deps.Err),
	}.Run(target, deps.Runtime)
	return output.Render(res, format, output.PlainView, deps)
}

// pushRepo returns a body that pushes one repository into its result lines.
func pushRepo(opts git.PushOptions, deps app.RuntimeCLI) func(
	context.Context, run.Repo, service.Runtime,
) (string, error) {
	return func(ctx context.Context, repo run.Repo, rt service.Runtime) (string, error) {
		// A ref-selecting flag already says which refs to push, so the
		// Upstream lookup — and the skip hanging off it — is suspended and
		// git resolves the destination itself.
		if opts.SelectsRefs() {
			out, err := rt.Git.Push(ctx, repo.AbsPath, git.PushTarget{}, opts)
			if err != nil {
				return "", err
			}
			return deps.Theme.GitOutput.Render(out), nil
		}

		// The branch and its Upstream arrive together, from git's own reading
		// of whether that Upstream resolves — as they do for `pull`. `status`
		// derives the same Gone Upstream state from its working-tree snapshot
		// instead (Snapshot.GoneUpstream); a fix to one belongs in the other.
		head, err := rt.Git.HeadUpstream(ctx, repo.AbsPath)
		if err != nil {
			return "", err
		}

		switch {
		case head.Upstream == "":
			// Pushing a branch with no Upstream is undefined, and this command
			// is documented to pass over it — so it is wrapped here as a
			// warning, which the engine leaves alone: it shows on the line
			// without failing the run.
			return "", domain.NewWarning("skipped: %s", git.ErrNoUpstream)
		case head.Gone:
			// Pushing here would succeed and re-create the branch someone
			// deleted on the Remote — the creative, ref-scattering behavior
			// ADR-0002 exists to prevent — so it is passed over instead, with
			// the Upstream named to tell this skip from the one above.
			return "", domain.NewWarning("skipped: %s: %s", head.Upstream, git.ErrUpstreamGone)
		}

		remote, branch, ok := git.SplitUpstream(head.Upstream)
		if !ok {
			// A branch tracking another local branch has an Upstream that
			// resolves, so neither skip above applies, but there is nowhere to
			// push it.
			return "", fmt.Errorf("upstream %q is not on a remote", head.Upstream)
		}

		target := git.PushTarget{
			Remote:  remote,
			Refspec: head.Branch + ":" + branch,
		}
		out, err := rt.Git.Push(ctx, repo.AbsPath, target, opts)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf(
			"[%s -> %s] %s",
			head.Branch,
			head.Upstream,
			deps.Theme.GitOutput.Render(out),
		), nil
	}
}
