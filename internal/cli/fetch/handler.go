// Package fetch implements `gits fetch`, the Bulk Command that fetches and
// prunes every Remote of every Repository in a Project.
package fetch

import (
	"context"
	"fmt"

	"github.com/rafi/gits/internal/bulk"
	"github.com/rafi/gits/internal/cli"
	"github.com/rafi/gits/internal/types"
)

// ExecFetch runs fetch on project repositories, or on a specific repo.
//
// Args: (optional)
//   - project name
//   - repo or sub-project name
func ExecFetch(args []string, deps types.RuntimeCLI) error {
	res, err := bulk.Command[string]{
		Verb: "fetching",
		Body: fetchRepo,
	}.Run(args, deps)
	if err != nil {
		return err
	}
	return bulk.Lines(res, deps)
}

// fetchRepo fetches one repository and returns its result line's body.
func fetchRepo(ctx context.Context, repo bulk.Repo, deps types.RuntimeCLI) (string, error) {
	output, err := deps.Git.Fetch(ctx, repo.AbsPath)
	if err != nil {
		return "", err
	}
	body := deps.Theme.GitOutput.Render(output)
	if repoPath := cli.Path(repo.AbsPath, deps.HomeDir); repo.Path != repoPath {
		body = fmt.Sprintf("%s %s", repoPath, body)
	}
	return body, nil
}
