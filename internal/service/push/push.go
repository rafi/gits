// Package push pushes one repository's current branch to its upstream,
// reporting what it found rather than a line to print.
// Bulk push is safe by construction.
package push

import (
	"context"
	"fmt"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/infra/git"
	coreruntime "github.com/rafi/gits/internal/runtime"
	"github.com/rafi/gits/internal/runtime/command"
)

// Report is what pushing one repository produced.
type Report struct {
	// Branch is the branch that was pushed, and Upstream the ref it was
	// pushed to. Both are empty when a ref-selecting flag let git choose the
	// destination itself, so there is no single branch to name.
	Branch   string
	Upstream string
	// Output is git's own output, unstyled.
	Output string
}

// Repo pushes one repository. A branch with nowhere to push to is a
// documented pass-over, returned as a warning so it shows on the
// repository's line without failing the command.
func Repo(
	ctx context.Context, repo command.Repo, opts git.PushOptions, rt coreruntime.Runtime,
) (Report, error) {
	// A ref-selecting flag already says which refs to push, so the upstream
	// lookup — and the skip hanging off it — is suspended and git resolves
	// the destination itself.
	if opts.SelectsRefs() {
		out, err := rt.Git.Push(ctx, repo.AbsPath, git.PushTarget{}, opts)
		if err != nil {
			return Report{}, err
		}
		return Report{Output: out}, nil
	}

	// The branch and its Upstream arrive together, from git's own reading of
	// whether that Upstream resolves — as they do for `pull`. `status`
	// derives the same Gone Upstream state from its working-tree snapshot
	// instead (Snapshot.GoneUpstream); a fix to one belongs in the other.
	head, err := rt.Git.HeadUpstream(ctx, repo.AbsPath)
	if err != nil {
		return Report{}, err
	}

	switch {
	case head.Upstream == "":
		// Pushing a branch with no Upstream is undefined, and this command is
		// documented to pass over it — so it is wrapped here as a warning,
		// which the engine leaves alone: it shows on the line without failing
		// the command.
		return Report{}, domain.NewWarning("skipped: %s", git.ErrNoUpstream)
	case head.Gone:
		// Pushing here would succeed and re-create the branch someone deleted
		// on the Remote. Bulk push must not recreate deleted branches, so this
		// Repository is passed over with the Upstream named to distinguish it
		// from the no-Upstream skip above.
		return Report{}, domain.NewWarning("skipped: %s: %s", head.Upstream, git.ErrUpstreamGone)
	}

	remote, branch, ok := git.SplitUpstream(head.Upstream)
	if !ok {
		// A branch tracking another local branch has an Upstream that
		// resolves, so neither skip above applies, but there is nowhere to
		// push it.
		return Report{}, fmt.Errorf("upstream %q is not on a remote", head.Upstream)
	}

	target := git.PushTarget{Remote: remote, Refspec: head.Branch + ":" + branch}
	out, err := rt.Git.Push(ctx, repo.AbsPath, target, opts)
	if err != nil {
		return Report{}, err
	}
	return Report{Branch: head.Branch, Upstream: head.Upstream, Output: out}, nil
}
