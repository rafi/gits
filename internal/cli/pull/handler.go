package pull

import (
	"errors"
	"fmt"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cli"
	"github.com/rafi/gits/internal/types"
)

// ExecPull runs pull --ff-only on project repositories, or on a specific repo.
//
// Args: (optional)
//   - project name
//   - repo
func ExecPull(args []string, deps types.RuntimeCLI) error {
	project, repo, err := cli.ParseArgs(args, true, deps)
	if err != nil {
		return err
	}

	if repo != nil {
		// Pull a single repository.
		return pullRepo(project, *repo, deps)
	}

	// Pull all project's repositories.
	errs := pullProjectRepos(project, deps)
	if len(errs) > 0 {
		cli.RenderErrors(errs, true)
		return errors.New("pull completed with errors")
	}
	return nil
}

func pullProjectRepos(project domain.Project, deps types.RuntimeCLI) []error {
	fmt.Println(cli.ProjectTitleWithBullet(project, deps.Theme))

	errList := make([]error, 0)
	for _, repo := range project.Repos {
		err := pullRepo(project, repo, deps)
		if err != nil {
			errList = append(errList, err)
		}
	}
	for _, subProject := range project.SubProjects {
		fmt.Println()
		errs := pullProjectRepos(subProject, deps)
		errList = append(errList, errs...)
	}
	return errList
}

func pullRepo(project domain.Project, repo domain.Repository, deps types.RuntimeCLI) error {
	maxLen := cli.GetMaxLen(project)
	repoTitle := cli.RepoTitle(project, repo, deps.HomeDir, deps.Theme).
		Width(maxLen)

	fmt.Printf("%s ", repoTitle)
	defer fmt.Println()

	// Abort if repository is not cloned or has errors.
	if repo.State != domain.RepoStateOK {
		return cli.AbortOnRepoState(repo, deps.Theme.Error)
	}

	gitRepo, err := deps.Git.Open(repo.AbsPath)
	if err != nil {
		return cli.RepoError(err, repo)
	}

	currentBranch, err := gitRepo.CurrentBranch()
	if err != nil {
		return cli.RepoError(err, repo)
	}

	upstream, err := gitRepo.GetUpstream(currentBranch)
	if err != nil {
		return cli.RepoError(err, repo)
	}

	fmt.Printf("[%s <- %s] ", currentBranch, upstream.Short())

	out, err := deps.Git.Pull(repo.AbsPath)
	fmt.Print(deps.Theme.GitOutput.Render(out))
	if err != nil {
		fmt.Print(deps.Theme.Error.Render(err.Error()))
		return cli.RepoError(err, repo)
	}
	return nil
}
