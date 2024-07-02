package fetch

import (
	"fmt"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cli"
	"github.com/rafi/gits/internal/types"
)

// ExecFetch runs fetch on project repositories, or on a specific repo.
//
// Args: (optional)
//   - project name
//   - repo or sub-project name
func ExecFetch(args []string, deps types.RuntimeCLI) error {
	project, repo, err := cli.ParseArgs(args, true, deps)
	if err != nil {
		return err
	}

	if repo != nil {
		// Fetch a single repository.
		return fetchRepo(project, *repo, deps)
	}

	// Fetch all project's repositories.
	errs := fetchProjectRepos(project, deps)
	if len(errs) > 0 {
		return cli.RenderErrors(errs, true)
	}
	return nil
}

func fetchProjectRepos(project domain.Project, deps types.RuntimeCLI) []error {
	fmt.Println(cli.ProjectTitleWithBullet(project, deps.Theme))

	errList := make([]error, 0)
	for _, repo := range project.Repos {
		err := fetchRepo(project, repo, deps)
		if err != nil {
			errList = append(errList, err)
		}
	}
	for _, subProject := range project.SubProjects {
		fmt.Println()
		errs := fetchProjectRepos(subProject, deps)
		errList = append(errList, errs...)
	}
	return errList
}

func fetchRepo(project domain.Project, repo domain.Repository, deps types.RuntimeCLI) error {
	maxLen := cli.GetMaxLen(project)
	repoTitle := cli.RepoTitle(project, repo, deps.HomeDir, deps.Theme).
		Width(maxLen)

	repoPath := cli.Path(repo.AbsPath, deps.HomeDir)
	if repoTitle.Value() == repoPath {
		fmt.Printf("%s ", repoTitle)
	} else {
		fmt.Printf("%s %s ", repoTitle, repoPath)
	}
	defer fmt.Println()

	// Abort if repository is not cloned or has errors.
	if repo.State != domain.RepoStateOK {
		return cli.AbortOnRepoState(repo, deps.Theme.Error)
	}

	out, err := deps.Git.Fetch(repo.AbsPath)
	fmt.Print(deps.Theme.GitOutput.Render(out))
	if err != nil {
		fmt.Print(deps.Theme.Error.Render(err.Error()))
		return cli.RepoError(err, repo)
	}
	return nil
}
