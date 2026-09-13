// Package fetch implements `gits fetch`, the Bulk Command that fetches and
// prunes every Remote of every Repository in a Project.
package fetch

import (
	"context"
	"fmt"

	"github.com/rafi/gits/internal/app"
	"github.com/rafi/gits/internal/app/format"
	"github.com/rafi/gits/internal/bulk"
)

// ExecFetch runs fetch on project repositories, or on a specific repo,
// rendering the results as lines or as the JSON envelope.
//
// Args: (optional)
//   - project name
//   - repo or sub-project name
func ExecFetch(format string, args []string, deps app.RuntimeCLI) error {
	// Validate before anything is loaded or selected, so a typo'd format never
	// costs a provider round-trip or an interactive prompt.
	if err := bulk.ValidateFormat(format); err != nil {
		return err
	}

	res, err := bulk.Command[string]{
		Name: "fetch",
		Verb: "fetching",
		Body: fetchRepo,
	}.Run(args, deps)
	if err != nil {
		return err
	}
	return bulk.Render(res, format, deps)
}

// fetchRepo fetches one repository and returns its result line's body.
func fetchRepo(ctx context.Context, repo bulk.Repo, deps app.RuntimeCLI) (string, error) {
	output, err := deps.Git.Fetch(ctx, repo.AbsPath)
	if err != nil {
		return "", err
	}
	body := deps.Theme.GitOutput.Render(output)
	if repoPath := format.Path(repo.AbsPath, deps.HomeDir); repo.Path != repoPath {
		body = fmt.Sprintf("%s %s", repoPath, body)
	}
	return body, nil
}
