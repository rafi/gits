package status

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cli"
	"github.com/rafi/gits/internal/cli/types"
)

// ExecStatus displays an icon based status of all repositories.
// Runs 'git' directly due to https://github.com/go-git/go-git/issues/181
//
// Args: (optional)
//   - project name
//   - repo or sub-project name
func ExecStatus(include []string, deps types.RuntimeDeps) error {
	project, err := cli.GetOrSelectProject(include, deps)
	if err != nil {
		return err
	}

	if len(include) > 1 && strings.Index(include[1], "/") > 0 {
		include = include[:len(include)-1]
	}

	var errList []cli.Error
	statusProject(project, deps, &errList)
	cli.HandlerErrors(errList)
	return nil
}

func statusProject(project domain.Project, deps types.RuntimeDeps, errList *[]cli.Error) {
	fmt.Println(cli.ProjectTitleWithBullet(project, deps.Theme))
	maxLen := cli.GetMaxLen(project)

	for _, repo := range project.Repos {
		repoTitle := cli.RepoTitle(project, repo, deps.HomeDir).
			Inherit(deps.Theme.RepoTitle).
			MarginLeft(types.LeftMargin).MarginRight(types.RightMargin).
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

func statusRepo(repoTitle string, repo domain.Repository, deps types.RuntimeDeps, errList *[]cli.Error) {
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

	branch, err := deps.Git.CurrentBranch(repo.AbsPath)
	if err != nil {
		fmt.Println(err)
	}
	upstream, err := deps.Git.UpstreamBranch(repo.AbsPath)
	if err != nil {
		upstream = "ERROR"
		fmt.Println(err)
	}
	if upstream == "" {
		upstream = fmt.Sprintf("origin/%v", branch)
	}

	diff := ""
	ahead, behind, err := deps.Git.Diff(repo.AbsPath, branch, upstream)
	if err != nil {
		diff = "-"
	}
	if ahead == 0 && behind == 0 {
		diff = "✓"
	}
	if ahead > 0 {
		diff = fmt.Sprintf("▲%d", ahead)
	}
	if behind > 0 {
		diff = fmt.Sprintf("%s▼%d", diff, behind)
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
