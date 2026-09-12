// Package push implements `gits push`, the Bulk Command that pushes each
// Repository's current branch to its Upstream.
// See: docs/adr/0002-push-safety-model.md.
package push

import (
	"context"
	"fmt"

	"github.com/rafi/gits/internal/bulk"
	"github.com/rafi/gits/internal/git"
	"github.com/rafi/gits/internal/types"
)

// ExecPush pushes project repositories, or a specific repo, to their
// Upstream. See docs/adr/0002-push-safety-model.md for what it deliberately
// cannot do.
//
// Args: (optional)
//   - project name
//   - repo
func ExecPush(opts git.PushOptions, args []string, deps types.RuntimeCLI) error {
	// Validate before anything is loaded or selected, so a rejected flag
	// combination costs neither a provider round-trip nor a single remote.
	if err := opts.Validate(); err != nil {
		return err
	}

	res, err := bulk.Command[string]{
		Verb: "pushing",
		Body: pushRepo(opts),
	}.Run(args, deps)
	if err != nil {
		return err
	}
	return bulk.Lines(res, deps)
}

// pushRepo returns a body that pushes one repository into its result lines.
func pushRepo(opts git.PushOptions) func(
	context.Context, bulk.Repo, types.RuntimeCLI,
) (string, error) {
	return func(ctx context.Context, repo bulk.Repo, deps types.RuntimeCLI) (string, error) {
		// A ref-selecting flag already says which refs to push, so the
		// Upstream lookup — and the skip hanging off it — is suspended and
		// git resolves the destination itself.
		if opts.SelectsRefs() {
			output, err := deps.Git.Push(ctx, repo.AbsPath, git.PushTarget{}, opts)
			if err != nil {
				return "", err
			}
			return deps.Theme.GitOutput.Render(output), nil
		}

		// The branch and its Upstream arrive together, from git's own reading
		// of whether that Upstream resolves — as they do for `pull`. `status`
		// derives the same Gone Upstream state from its working-tree snapshot
		// instead (Snapshot.GoneUpstream); a fix to one belongs in the other.
		head, err := deps.Git.HeadUpstream(ctx, repo.AbsPath)
		if err != nil {
			return "", err
		}

		switch {
		case head.Upstream == "":
			// Pushing a branch with no Upstream is undefined, and this command
			// is documented to pass over it — so it is wrapped here as a
			// warning, which the module leaves alone: it shows on the line
			// without failing the run.
			return "", types.NewWarning("skipped: %s", git.ErrNoUpstream)
		case head.Gone:
			// Pushing here would succeed and re-create the branch someone
			// deleted on the Remote — the creative, ref-scattering behavior
			// ADR-0002 exists to prevent — so it is passed over instead, with
			// the Upstream named to tell this skip from the one above.
			return "", types.NewWarning("skipped: %s: %s", head.Upstream, git.ErrUpstreamGone)
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
		output, err := deps.Git.Push(ctx, repo.AbsPath, target, opts)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf(
			"[%s -> %s] %s",
			head.Branch,
			head.Upstream,
			deps.Theme.GitOutput.Render(output),
		), nil
	}
}
