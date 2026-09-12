// Package pull implements `gits pull`, the Bulk Command that fast-forwards
// every Repository in a Project.
package pull

import (
	"context"
	"fmt"

	"github.com/rafi/gits/internal/bulk"
	"github.com/rafi/gits/internal/git"
	"github.com/rafi/gits/internal/types"
)

// ExecPull runs pull --ff-only on project repositories, or on a specific
// repo, rendering the results as lines or as the JSON envelope.
//
// Args: (optional)
//   - project name
//   - repo
func ExecPull(format string, args []string, deps types.RuntimeCLI) error {
	// Validate before anything is loaded or selected, so a typo'd format never
	// costs a provider round-trip or an interactive prompt.
	if err := bulk.ValidateFormat(format); err != nil {
		return err
	}

	res, err := bulk.Command[string]{
		Name: "pull",
		Verb: "pulling",
		Body: pullRepo,
	}.Run(args, deps)
	if err != nil {
		return err
	}
	return bulk.Render(res, format, deps)
}

// pullRepo pulls one repository and returns its result line's body.
func pullRepo(ctx context.Context, repo bulk.Repo, deps types.RuntimeCLI) (string, error) {
	head, err := deps.Git.HeadUpstream(ctx, repo.AbsPath)
	if err != nil {
		return "", err
	}

	switch {
	case head.Upstream == "":
		// There is nowhere to pull from
		return "", types.NewWarning("skipped: %s", git.ErrNoUpstream)
	case head.Gone:
		// Branch is gone, merged and deleted?
		return "", types.NewWarning("skipped: %s: %s", head.Upstream, git.ErrUpstreamGone)
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
