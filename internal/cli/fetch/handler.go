package fetch

import (
	"fmt"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cli"
	"github.com/rafi/gits/internal/project"
)

func ExecFetch(include []string, deps cli.RuntimeDeps) error {
	projects, err := project.GetProjects(include, deps)
	if err != nil {
		return fmt.Errorf("unable to list projects: %w", err)
	}

	var errList []cli.Error
	for _, project := range projects {
		fetchProject(project, deps, &errList)
	}
	cli.HandlerErrors(errList)
	return nil
}

func fetchProject(project domain.Project, deps cli.RuntimeDeps, errList *[]cli.Error) {
	fmt.Println(cli.ProjectTitle(project, deps.Theme))
	maxLen := cli.GetMaxLen(project)

	for _, repo := range project.Repos {
		repoTitle := cli.RepoTitle(project, repo, deps.HomeDir).
			Inherit(deps.Theme.RepoTitle).
			MarginLeft(cli.LeftMargin).MarginRight(cli.RightMargin).
			Width(maxLen).
			Render()
		fetchRepo(repoTitle, repo, deps, errList)
	}
	for _, subProject := range project.SubProjects {
		fmt.Println()
		fetchProject(subProject, deps, errList)
	}
}

func fetchRepo(repoTitle string, repo domain.Repository, deps cli.RuntimeDeps, errList *[]cli.Error) {
	repoPath := cli.Path(repo.AbsPath, deps.HomeDir)
	fmt.Printf("%s %s ", repoTitle, repoPath)
	errorStyle := deps.Theme.Error.Copy().PaddingLeft(1)

	switch repo.State {
	case domain.RepoStateError:
		fmt.Printf("%s\n", errorStyle.Render("Not a Git repository"))
		return
	case domain.RepoStateNoLocal:
		fmt.Printf("%s\n", errorStyle.Render("Not cloned"))
		return
	}

	output, err := deps.Git.Fetch(repo.AbsPath)
	if err != nil {
		*errList = append(*errList, cli.Error{
			Message: fmt.Sprint(err),
			Title:   repo.GetName(),
			Dir:     repoPath,
		})
		output = errorStyle.Render(err.Error())
	}
	fmt.Println(deps.Theme.GitOutput.Render(output))
}
