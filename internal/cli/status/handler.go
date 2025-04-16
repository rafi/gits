package status

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cli"
	"github.com/rafi/gits/internal/types"
)

// ExecStatus displays an icon based status of all repositories.
// Runs 'git' directly due to https://github.com/go-git/go-git/issues/181
//
// Args: (optional)
//   - project name
//   - repo or sub-project name
func ExecStatus(args []string, deps types.RuntimeCLI) error {
	project, repo, err := cli.ParseArgs(args, true, deps)
	if err != nil {
		return err
	}

	if repo != nil {
		// Display status for a single repository.
		title := cli.RepoTitle(*repo, project.AbsPath, deps.HomeDir, deps.Theme).Align(lipgloss.Right)
		fmt.Printf("%s ", title)
		return statusRepo(*repo, deps)
	}

	// Display status for all project's repositories.
	errs := statusProject(project, deps)
	if len(errs) > 0 {
		return cli.RenderErrors(errs, true)
	}
	return nil
}

func statusProject(project domain.Project, deps types.RuntimeCLI) []error {
	fmt.Println(cli.ProjectTitleWithBullet(project, deps.Theme))
	maxLen := cli.GetMaxLen(project)

	errList := make([]error, 0)
	for _, repo := range project.Repos {
		repoTitle := cli.RepoTitle(repo, project.AbsPath, deps.HomeDir, deps.Theme).
			Width(maxLen).
			Align(lipgloss.Right).
			Render()

		fmt.Printf("%s ", repoTitle)
		err := statusRepo(repo, deps)
		if err != nil {
			errList = append(errList, err)
		}
	}
	for _, subProject := range project.SubProjects {
		fmt.Println()
		errs := statusProject(subProject, deps)
		errList = append(errList, errs...)
	}
	return errList
}

func statusRepo(repo domain.Repository, deps types.RuntimeCLI) error {
	defer fmt.Println()

	// Abort if repository is not cloned or has errors.
	if repo.State != domain.RepoStateOK {
		errStyle := deps.Theme.Error.PaddingLeft(8)
		return cli.AbortOnRepoState(repo, errStyle)
	}

	version, err := deps.Git.Describe(repo.AbsPath)
	if err != nil {
		version = ""
	}

	var count int
	modified := ""
	if count, err = deps.Git.Modified(repo.AbsPath); err != nil {
		return cli.RepoError(err, repo)
	} else if count > 0 {
		modified = fmt.Sprintf("≠%d", count)
	}
	untracked := ""
	if count, err = deps.Git.Untracked(repo.AbsPath); err != nil {
		return cli.RepoError(err, repo)
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

	fmt.Printf("%s %s %s %s %s",
		deps.Theme.Modified.Render(modified),
		deps.Theme.Untracked.Render(untracked),
		deps.Theme.Diff.Render(diff),
		version,
		currentRef,
	)
	return nil
}
