package push

import (
	"context"
	"errors"
	"fmt"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cli"
	"github.com/rafi/gits/internal/cli/walk"
	"github.com/rafi/gits/internal/types"
	"github.com/rafi/gits/pkg/git"
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

	project, repo, err := cli.ParseArgs(args, true, deps)
	if err != nil {
		return err
	}

	fn := pushRepo(cli.NewTitleWidths(project, deps.HomeDir), opts)

	if repo != nil {
		return walk.Single(deps.Ctx, project, *repo, deps, fn)
	}

	// Push all project repositories through the shared walker.
	errs := walk.Walk(deps.Ctx, project, deps, "pushing", fn)
	return cli.RenderErrors(deps.Err, errs, true)
}

// pushRepo returns a walk.RepoFunc that pushes one repository into a rendered
// result: safe to call concurrently and never writes to stdout.
func pushRepo(widths cli.TitleWidths, opts git.PushOptions) walk.RepoFunc {
	return func(
		ctx context.Context,
		project domain.Project,
		repo domain.Repository,
		deps types.RuntimeCLI,
	) walk.RepoResult {
		line := cli.RepoLine{
			Title:      cli.PaddedRepoTitle(repo, project, widths.For(project), deps),
			ErrorStyle: deps.Theme.Error,
		}

		// Abort if repository is not cloned or has errors.
		if repo.State != domain.RepoStateOK {
			return walk.LineResult(line, cli.RepoStateError(repo))
		}

		// A ref-selecting flag already says which refs to push, so the
		// Upstream lookup — and the skip hanging off it — is suspended and
		// git resolves the destination itself.
		if opts.SelectsRefs() {
			output, err := deps.Git.Push(ctx, repo.AbsPath, git.PushTarget{}, opts)
			if err != nil {
				return walk.LineResult(line, cli.RepoError(err, repo))
			}
			line.Body = deps.Theme.GitOutput.Render(output)
			return walk.LineResult(line, nil)
		}

		currentBranch, err := deps.Git.CurrentBranch(ctx, repo.AbsPath)
		if err != nil {
			return walk.LineResult(line, cli.RepoError(err, repo))
		}

		upstream, err := deps.Git.UpstreamBranch(ctx, repo.AbsPath)
		if err != nil && !errors.Is(err, git.ErrNoUpstream) {
			// A real failure (e.g. cancellation) keeps its own message instead
			// of being mislabeled as a missing upstream.
			return walk.LineResult(line, cli.RepoError(err, repo))
		}
		if err != nil || upstream == "" {
			// Pushing a branch with no Upstream is undefined, and this command
			// is documented to pass over it — so it is a warning, which shows
			// on the line without failing the run.
			return walk.LineResult(line, cli.RepoWarning(
				fmt.Errorf("skipped: %w", git.ErrNoUpstream), repo))
		}

		remote, branch, ok := git.SplitUpstream(upstream)
		if !ok {
			// A branch tracking another local branch has an Upstream, so the
			// skip above does not apply, but there is nowhere to push it.
			return walk.LineResult(line, cli.RepoError(
				fmt.Errorf("upstream %q is not on a remote", upstream), repo))
		}

		target := git.PushTarget{
			Remote:  remote,
			Refspec: currentBranch + ":" + branch,
		}
		output, err := deps.Git.Push(ctx, repo.AbsPath, target, opts)
		if err != nil {
			return walk.LineResult(line, cli.RepoError(err, repo))
		}
		line.Body = fmt.Sprintf(
			"[%s -> %s] %s",
			currentBranch,
			upstream,
			deps.Theme.GitOutput.Render(output),
		)
		return walk.LineResult(line, nil)
	}
}
