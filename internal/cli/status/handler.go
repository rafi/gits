package status

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cli"
	"github.com/rafi/gits/internal/project"
)

// ExecStatus displays an icon based status of all repositories.
// Runs 'git' directly due to https://github.com/go-git/go-git/issues/181
func ExecStatus(include []string, deps cli.RuntimeDeps) error {
	projects, err := project.GetProjects(include, deps)
	if err != nil {
		return fmt.Errorf("unable to list projects: %w", err)
	}

	var errList []cli.Error
	for _, project := range projects {
		statusProject(project, deps, &errList)
	}
	cli.HandlerErrors(errList)
	return nil
}

func statusProject(project domain.Project, deps cli.RuntimeDeps, errList *[]cli.Error) {
	fmt.Println(cli.ProjectTitle(project, deps.Theme))
	maxLen := cli.GetMaxLen(project)

	for _, repo := range project.Repos {
		repoTitle := cli.RepoTitle(project, repo, deps.HomeDir).
			Inherit(deps.Theme.RepoTitle).
			MarginLeft(cli.LeftMargin).MarginRight(cli.RightMargin).
			Align(lipgloss.Right).
			Width(maxLen).
			Render()
		statusRepo(repoTitle, repo, deps, errList)
	}
	for _, subProject := range project.SubProjects {
		fmt.Println()
		statusProject(subProject, deps, errList)
	}
}

func statusRepo(repoTitle string, repo domain.Repository, deps cli.RuntimeDeps, errList *[]cli.Error) {
	errorStyle := deps.Theme.Error.Copy().PaddingLeft(14)
	fmt.Printf("%s ", repoTitle)

	switch repo.State {
	case domain.RepoStateError:
		fmt.Printf("%s\n", errorStyle.Render("Not a Git repository"))
		return
	case domain.RepoStateNoLocal:
		fmt.Printf("%s\n", errorStyle.Render("Not cloned"))
		return
	}

	version, err := deps.Git.Describe(repo.AbsPath)
	if err != nil {
		version = ""
	}

	var count int
	modified := ""
	if count, err = deps.Git.Modified(repo.AbsPath); err != nil {
		*errList = append(*errList, cli.Error{
			Message: fmt.Sprint(err),
			Title:   repo.GetName(),
			Dir:     repo.AbsPath,
		})
		return
	} else if count > 0 {
		modified = fmt.Sprintf("≠%d", count)
	}
	untracked := ""
	if count, err = deps.Git.Untracked(repo.AbsPath); err != nil {
		*errList = append(*errList, cli.Error{
			Message: fmt.Sprint(err),
			Title:   repo.GetName(),
			Dir:     repo.AbsPath,
		})
		return
	} else if count > 0 {
		untracked = fmt.Sprintf("?%d", count)
	}

	diff, err := deps.Git.Diff(repo.AbsPath)
	if err != nil {
		diff = "-"
	}
	currentRef, err := deps.Git.CurrentPosition(repo.AbsPath)
	if err != nil {
		currentRef = "N/A"
	}

	fmt.Printf("%s %s %s %s %s\n",
		deps.Theme.Modified.Render(modified),
		deps.Theme.Untracked.Render(untracked),
		deps.Theme.Diff.Render(diff),
		version,
		currentRef,
	)
}
