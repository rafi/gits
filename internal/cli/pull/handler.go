package pull

import (
	"fmt"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cli"
	"github.com/rafi/gits/internal/types"
)

// ExecPull runs pull --no-ff on project repositories, or on a specific repo.
//
// Args: (optional)
//   - project name
//   - repo
func ExecPull(args []string, deps types.RuntimeCLI) error {
	project, repo, err := cli.ParseArgs(args, false, deps)
	if err != nil {
		return err
	}

	// Fetch all project's repositories.
	if repo == nil {
		var errList []cli.Error
		pullProjectRepos(project, deps, &errList)
		cli.HandlerErrors(errList)
		return nil
	}

	return pullRepo(project, *repo, deps)
}

func pullProjectRepos(project domain.Project, deps types.RuntimeCLI, errList *[]cli.Error) {
	errorStyle := deps.Theme.Error.Copy().PaddingLeft(1)

	fmt.Println(cli.ProjectTitleWithBullet(project, deps.Theme))

	for _, repo := range project.Repos {
		err := pullRepo(project, repo, deps)
		if err != nil {
			*errList = append(*errList, cli.Error{
				Message: fmt.Sprint(err),
				Title:   repo.GetName(),
				Dir:     repo.AbsPath,
			})
			errorMsg := errorStyle.Render(err.Error())
			fmt.Println(deps.Theme.GitOutput.Render(errorMsg))
		}
	}
	for _, subProject := range project.SubProjects {
		fmt.Println()
		pullProjectRepos(subProject, deps, errList)
	}
}

func pullRepo(project domain.Project, repo domain.Repository, deps types.RuntimeCLI) error {
	repoTitle := cli.RepoTitle(project, repo, deps.HomeDir).
		Inherit(deps.Theme.RepoTitle).
		MarginLeft(cli.LeftMargin).MarginRight(cli.RightMargin).
		Render()

	repoPath := cli.Path(repo.AbsPath, deps.HomeDir)
	fmt.Printf("%s %s ", repoTitle, repoPath)

	// Abort if repository is not cloned or has errors.
	if repo.State != domain.RepoStateOK {
		return cli.AbortOnRepoState(repo, deps.Theme)
	}

	gitRepo, err := deps.Git.Open(repo.AbsPath)
	if err != nil {
		return err
	}

	currentBranch, err := gitRepo.CurrentBranch()
	if err != nil {
		return err
	}

	upstream, err := gitRepo.GetUpstream(currentBranch)
	if err != nil {
		return err
	}

	fmt.Printf(" %s <- %s", currentBranch, upstream.Short())

	out, err := deps.Git.Pull(repo.AbsPath)
	fmt.Print(deps.Theme.GitOutput.Render(out))
	if err != nil {
		fmt.Print(deps.Theme.Error.Render(err.Error()))
	}
	return err
}
