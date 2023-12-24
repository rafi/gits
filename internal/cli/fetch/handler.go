package fetch

import (
	"fmt"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cli"
	"github.com/rafi/gits/internal/cli/types"
)

// ExecFetch runs fetch on project repositories, or on a specific repo.
//
// Args: (optional)
//   - project name
//   - repo name
func ExecFetch(args []string, deps types.RuntimeDeps) error {
	project, err := cli.GetOrSelectProject(args, deps)
	if err != nil {
		return err
	}

	// Get specific repo if provided/selected, or all repos in project.
	repos, err := cli.GetOrSelectRepos(project, args, deps)
	if err != nil {
		return err
	}

	if repos == nil {
		var errList []cli.Error
		fetchProjectRepos(project, deps, &errList)
		cli.HandlerErrors(errList)
		return nil
	}

	output, err := fetchRepo(project, repos[0], deps)
	fmt.Println(deps.Theme.GitOutput.Render(output))
	return err
}

func fetchProjectRepos(project domain.Project, deps types.RuntimeDeps, errList *[]cli.Error) {
	errorStyle := deps.Theme.Error.Copy().PaddingLeft(1)

	fmt.Println(cli.ProjectTitleWithBullet(project, deps.Theme))

	for _, repo := range project.Repos {
		output, err := fetchRepo(project, repo, deps)
		if err != nil {
			*errList = append(*errList, cli.Error{
				Message: fmt.Sprint(err),
				Title:   repo.GetName(),
				Dir:     repo.AbsPath,
			})
			output = errorStyle.Render(err.Error())
		}
		fmt.Println(deps.Theme.GitOutput.Render(output))
	}
	for _, subProject := range project.SubProjects {
		fmt.Println()
		fetchProjectRepos(subProject, deps, errList)
	}
}

func fetchRepo(project domain.Project, repo domain.Repository, deps types.RuntimeDeps) (string, error) {
	maxLen := cli.GetMaxLen(project)
	repoTitle := cli.RepoTitle(project, repo, deps.HomeDir).
		Inherit(deps.Theme.RepoTitle).
		MarginLeft(types.LeftMargin).MarginRight(types.RightMargin).
		Width(maxLen).
		Render()

	repoPath := cli.Path(repo.AbsPath, deps.HomeDir)
	fmt.Printf("%s %s ", repoTitle, repoPath)

	// Abort if repository is not cloned or has errors.
	if repo.State != domain.RepoStateOK {
		return "", cli.AbortOnRepoState(repo, deps.Theme)
	}

	return deps.Git.Fetch(repo.AbsPath)
}
