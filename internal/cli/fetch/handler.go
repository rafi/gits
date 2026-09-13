// Package fetch implements `gits fetch`, the Bulk Command that fetches and
// prunes every Remote of every Repository in a Project.
package fetch

import (
	"context"
	"fmt"

	"github.com/rafi/gits/internal/app"
	"github.com/rafi/gits/internal/app/cli/output"
	"github.com/rafi/gits/internal/app/cli/pick"
	"github.com/rafi/gits/internal/app/cli/progress"
	"github.com/rafi/gits/internal/format"
	"github.com/rafi/gits/internal/service"
	"github.com/rafi/gits/internal/service/run"
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
		Name:     "fetch",
		Verb:     "fetching",
		Do:       fetchRepo(deps),
		Progress: progress.New(deps.Err),
	}.Run(target, deps.Runtime)
	return output.Render(res, format, deps)
}

// fetchRepo returns a body that fetches one repository into its result line.
func fetchRepo(deps app.RuntimeCLI) func(
	context.Context, run.Repo, service.Runtime,
) (string, error) {
	return func(ctx context.Context, repo run.Repo, rt service.Runtime) (string, error) {
		out, err := rt.Git.Fetch(ctx, repo.AbsPath)
		if err != nil {
			return "", err
		}
		body := deps.Theme.GitOutput.Render(out)
		if repoPath := format.Path(repo.AbsPath, rt.HomeDir); repo.Path != repoPath {
			body = fmt.Sprintf("%s %s", repoPath, body)
		}
		return body, nil
	}
}
