package clone

import (
	"context"
	"errors"

	"charm.land/lipgloss/v2"
	log "github.com/sirupsen/logrus"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cli"
	"github.com/rafi/gits/internal/cli/walk"
	"github.com/rafi/gits/internal/types"
	"github.com/rafi/gits/pkg/git"
)

// ExecClone clones project repositories, or a specific repo.
//
// Args: (optional)
//   - project name
//   - repo or sub-project name
func ExecClone(args []string, deps types.RuntimeCLI) error {
	project, repo, err := cli.ParseArgs(args, true, deps)
	if err != nil {
		return err
	}

	if repo != nil {
		// Clone a single repository.
		res := cloneRepo(deps.Ctx, project, *repo, deps)
		lipgloss.Println(cli.IndentMultiline(res.Line))
		return res.Err
	}

	// Clone all project repositories through the shared walker. Projects with
	// clone disabled are pruned first so the walker never queues their repos.
	errs := walk.Walk(deps.Ctx, pruneSkipped(project), deps, "cloning", cloneRepo)
	return cli.RenderErrors(errs, true)
}

// pruneSkipped returns a copy of the project tree with clone-disabled projects
// emptied of their repos and sub-projects, preserving the historical behavior
// where `clone: false` skips a project and everything beneath it. The original
// tree is left unmodified.
func pruneSkipped(p domain.Project) domain.Project {
	if p.Clone != nil && !*p.Clone {
		log.Warn("Skipping clone due to config")
		p.Repos = nil
		p.SubProjects = nil
		return p
	}
	subs := make([]domain.Project, len(p.SubProjects))
	for i := range p.SubProjects {
		subs[i] = pruneSkipped(p.SubProjects[i])
	}
	p.SubProjects = subs
	return p
}

// cloneRepo clones one repository and returns its rendered result. It is a
// walk.RepoFunc: safe to call concurrently and never writes to stdout. Unlike
// the other bulk commands it only rejects repos in an error state, since a
// not-yet-cloned repo is the expected input here.
func cloneRepo(
	ctx context.Context,
	project domain.Project,
	repo domain.Repository,
	deps types.RuntimeCLI,
) walk.RepoResult {
	line := cli.RepoLine{
		Title:      cli.PaddedRepoTitle(repo, project, deps),
		ErrorStyle: deps.Theme.Error,
	}

	if repo.State == domain.RepoStateError {
		line.Err = cli.RepoStateWarning(repo)
		return walk.RepoResult{Line: line.String(), Err: line.Err}
	}

	output, err := deps.Git.Clone(ctx, repo.Src, repo.AbsPath)
	if errors.Is(err, git.ErrTargetExists) {
		repoPath := cli.Path(repo.AbsPath, deps.HomeDir)
		line.Err = types.NewWarning("already cloned at %s", repoPath)
		return walk.RepoResult{Line: line.String(), Err: line.Err}
	}
	if err != nil {
		line.Err = cli.RepoError(err, repo)
		return walk.RepoResult{Line: line.String(), Err: line.Err}
	}
	line.Body = deps.Theme.GitOutput.Render(output)
	return walk.RepoResult{Line: line.String(), Err: nil}
}
