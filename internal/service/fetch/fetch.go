// Package fetch fetches and prunes every remote of one repository, reporting
// git's output rather than a line to print.
package fetch

import (
	"context"

	"github.com/rafi/gits/internal/format"
	"github.com/rafi/gits/internal/service"
	"github.com/rafi/gits/internal/service/run"
)

// Report is what fetching one repository produced.
type Report struct {
	// Output is git's own output, unstyled.
	Output string
	// RepoPath is the repository's absolute location, shortened with ~, and
	// set only when it differs from the display path its line is titled
	// with — a repository living outside its project's tree, whose real
	// location the title does not say.
	RepoPath string
}

// Repo fetches one repository.
func Repo(ctx context.Context, repo run.Repo, rt service.Runtime) (Report, error) {
	out, err := rt.Git.Fetch(ctx, repo.AbsPath)
	if err != nil {
		return Report{}, err
	}
	rep := Report{Output: out}
	if path := format.Path(repo.AbsPath, rt.HomeDir); repo.Path != path {
		rep.RepoPath = path
	}
	return rep, nil
}
