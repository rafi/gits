// Package pull implements `gits pull`, the Bulk Command that fast-forwards
// every Repository in a Project.
package pull

import (
	"context"
	"fmt"

	"github.com/rafi/gits/internal/bulk"
	"github.com/rafi/gits/internal/cli"
	"github.com/rafi/gits/internal/git"
	"github.com/rafi/gits/internal/types"
)

// ExecPull runs pull --ff-only on project repositories, or on a specific repo.
//
// Args: (optional)
//   - project name
//   - repo
func ExecPull(args []string, deps types.RuntimeCLI) error {
	return bulk.Command[string]{
		Verb:   "pulling",
		Body:   pullRepo,
		Render: bulk.Lines,
	}.Run(args, deps)
}

// pullRepo pulls one repository and returns its result line's body: safe to
// call concurrently and never writes to a destination.
func pullRepo(ctx context.Context, repo bulk.Repo, deps types.RuntimeCLI) (string, error) {
	// The branch and its Upstream arrive together, from git's own reading of
	// whether that Upstream resolves. `status` derives the same Gone Upstream
	// state from its working-tree snapshot instead — see Snapshot.GoneUpstream
	// — because it takes one anyway; a fix to one belongs in the other.
	head, err := deps.Git.HeadUpstream(ctx, repo.AbsPath)
	if err != nil {
		return "", err
	}

	switch {
	case head.Upstream == "":
		// There is nowhere to pull from, and `push` passes over the same
		// condition — so it is a warning, which the module leaves alone: it
		// shows on the line without failing the run.
		return "", cli.RepoWarning(
			fmt.Errorf("skipped: %w", git.ErrNoUpstream), repo.Repository)
	case head.Gone:
		// The ordinary end of a merged branch, not a broken Repository: the
		// Upstream is named so the user knows which one went away.
		return "", cli.RepoWarning(
			fmt.Errorf("skipped: %s: %w", head.Upstream, git.ErrUpstreamGone),
			repo.Repository)
	}

	output, err := deps.Git.Pull(ctx, repo.AbsPath)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(
		"[%s <- %s] %s",
		head.Branch,
		head.Upstream,
		deps.Theme.GitOutput.Render(output),
	), nil
}
