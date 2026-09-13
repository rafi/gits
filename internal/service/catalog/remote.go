package catalog

import (
	"context"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/infra/git"
)

// FillSources fills in the Repo Src of every `ok` repository in the tree that
// has none, reading it from the clone's git remote. It is the lazy counterpart
// to what classifyRepo once did on every command: only the commands that
// display Src — `list`'s table/wide SOURCE column and the JSON envelope — pay
// the subprocess, and a repository whose remote cannot be read keeps its row
// with the failure recorded as its Reason rather than failing the command.
func FillSources(ctx context.Context, gitClient git.Reader, project *domain.Project) {
	for idx := range project.SubProjects {
		FillSources(ctx, gitClient, &project.SubProjects[idx])
	}
	for idx := range project.Repos {
		fillRepoSource(ctx, gitClient, &project.Repos[idx])
	}
}

// FillSourcesKeyed does the same across a keyed project list. It exists
// separately because map values are not addressable, so each project has to be
// copied out, filled, and written back.
func FillSourcesKeyed(ctx context.Context, gitClient git.Reader, projects domain.ProjectListKeyed) {
	for name, proj := range projects {
		FillSources(ctx, gitClient, &proj)
		projects[name] = proj
	}
}

// fillRepoSource resolves one repository's Repo Src from its clone's remote,
// when it has none. Only an `ok` repository with a resolved local path is
// consulted; a failed lookup is recorded as the row's Reason, matching how the
// table's SOURCE column renders a repository with no source.
func fillRepoSource(ctx context.Context, gitClient git.Reader, r *domain.Repository) {
	if r.Src != "" || r.State != domain.RepoStateOK || r.AbsPath == "" {
		return
	}
	src, err := gitClient.Remote(ctx, r.AbsPath)
	if err != nil {
		r.Reason = err.Error()
		return
	}
	r.Src = src
}
