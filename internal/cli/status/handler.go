package status

import (
	"fmt"

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
func ExecStatus(args []string, deps types.RuntimeDeps) error {
	project, repo, err := cli.ParseArgs(args, true, deps)
	if err != nil {
		return err
	}

	// Display status for all project's repositories.
	if repo == nil {
		var errList []cli.Error
		statusProject(project, deps, &errList)
		cli.HandlerErrors(errList)
		return nil
	}

	// Display status for a single repository.
	fmt.Printf("%s ", getRepoStyle(project, *repo, deps).Render())
	output, err := statusRepo(*repo, deps)
	fmt.Println(deps.Theme.GitOutput.Render(output))
	return err
}

func getRepoStyle(project domain.Project, repo domain.Repository, deps types.RuntimeDeps) lipgloss.Style {
	return cli.RepoTitle(project, repo, deps.HomeDir).
		Inherit(deps.Theme.RepoTitle).
		MarginLeft(types.LeftMargin).
		MarginRight(types.RightMargin).
		Align(lipgloss.Right)
}

func statusProject(project domain.Project, deps types.RuntimeDeps, errList *[]cli.Error) {
	fmt.Println(cli.ProjectTitleWithBullet(project, deps.Theme))
	maxLen := cli.GetMaxLen(project)
	errorStyle := deps.Theme.Error.Copy().PaddingLeft(14)

	for _, repo := range project.Repos {
		repoTitle := getRepoStyle(project, repo, deps).
			Width(maxLen).
			Render()

		fmt.Printf("%s ", repoTitle)
		output, err := statusRepo(repo, deps)
		if err != nil {
			*errList = append(*errList, cli.Error{
				Message: fmt.Sprint(err),
				Title:   repo.GetName(),
				Dir:     repo.AbsPath,
			})
			output = errorStyle.Render(err.Error())
		}
		fmt.Println(output)
	}
	for _, subProject := range project.SubProjects {
		fmt.Println()
		statusProject(subProject, deps, errList)
	}
}

func statusRepo(repo domain.Repository, deps types.RuntimeDeps) (string, error) {
	switch repo.State {
	case domain.RepoStateError:
		return "", fmt.Errorf("not a git repository")
	case domain.RepoStateNoLocal:
		return "", fmt.Errorf("not cloned")
	}

	version, err := deps.Git.Describe(repo.AbsPath)
	if err != nil {
		version = ""
	}

	var count int
	modified := ""
	if count, err = deps.Git.Modified(repo.AbsPath); err != nil {
		return "", err
	} else if count > 0 {
		modified = fmt.Sprintf("≠%d", count)
	}
	untracked := ""
	if count, err = deps.Git.Untracked(repo.AbsPath); err != nil {
		return "", err
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

	return fmt.Sprintf("%s %s %s %s %s",
		deps.Theme.Modified.Render(modified),
		deps.Theme.Untracked.Render(untracked),
		deps.Theme.Diff.Render(diff),
		version,
		currentRef,
	), nil
}
