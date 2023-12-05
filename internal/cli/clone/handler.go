package clone

import (
	"fmt"
	"os"

	log "github.com/sirupsen/logrus"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cli"
	"github.com/rafi/gits/internal/project"
)

func ExecClone(include []string, deps cli.RuntimeDeps) error {
	projects, err := project.GetProjects(include, deps)
	if err != nil {
		return fmt.Errorf("unable to list projects: %w", err)
	}

	var errList []cli.Error
	for _, project := range projects {
		checkoutProject(project, deps, &errList)
	}
	cli.HandlerErrors(errList)
	return nil
}

func checkoutProject(project domain.Project, deps cli.RuntimeDeps, errList *[]cli.Error) {
	fmt.Println(cli.ProjectTitle(project, deps.Theme))
	if project.Clone != nil && !*project.Clone {
		log.Warn("Skipping clone due to config")
		return
	}
	if project.Path == "" {
		log.Warn("Skipping clone due to missing path")
		return
	}
	maxLen := cli.GetMaxLen(project)

	for _, repo := range project.Repos {
		repoTitle := cli.RepoTitle(project, repo, deps.HomeDir).
			Inherit(deps.Theme.RepoTitle).
			MarginLeft(cli.LeftMargin).MarginRight(cli.RightMargin).
			Width(maxLen).
			Render()
		checkoutRepo(repoTitle, repo, deps, errList)
	}
	for _, subProject := range project.SubProjects {
		fmt.Println()
		checkoutProject(subProject, deps, errList)
	}
}

func checkoutRepo(repoTitle string, repo domain.Repository, deps cli.RuntimeDeps, errList *[]cli.Error) {
	errorStyle := deps.Theme.Error.Copy()
	fmt.Printf("%s ", repoTitle)

	if repo.State == domain.RepoStateError {
		fmt.Printf("%s\n", errorStyle.Render(repo.Reason))
		return
	}
	repoPath := cli.Path(repo.AbsPath, deps.HomeDir)

	if _, err := os.Stat(repo.AbsPath); !os.IsNotExist(err) {
		fmt.Printf("%s %s\n", repoPath, errorStyle.Render("Directory already exists"))
		return
	}
	result, err := deps.Git.Clone(repo.Src, repo.AbsPath)
	if err != nil {
		*errList = append(*errList, cli.Error{
			Message: fmt.Sprint(err),
			Title:   repo.GetName(),
			Dir:     repoPath,
		})
		result = errorStyle.Render(err.Error())
	}
	fmt.Println(deps.Theme.GitOutput.Render(result))
}
