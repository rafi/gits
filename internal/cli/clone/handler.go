package clone

import (
	"fmt"
	"os"

	log "github.com/sirupsen/logrus"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cli"
	"github.com/rafi/gits/internal/types"
)

// ExecClone clones project repositories, or a specific repo.
//
// Args: (optional)
//   - project name
//   - repo or sub-project name
func ExecClone(args []string, deps types.RuntimeCLI) error {
	project, repo, err := cli.ParseArgs(args, true, deps)
	if err != nil {
		return err
	}

	if repo != nil {
		// Clone a single repository.
		return cloneRepo(project, *repo, deps)
	}

	// Clone all project's repositories.
	errs := cloneProjectRepos(project, deps)
	if len(errs) > 0 {
		return cli.RenderErrors(errs, true)
	}
	return nil
}

func cloneProjectRepos(project domain.Project, deps types.RuntimeCLI) []error {
	fmt.Println(cli.ProjectTitleWithBullet(project, deps.Theme))

	errList := make([]error, 0)
	if project.Clone != nil && !*project.Clone {
		log.Warn("Skipping clone due to config")
		return nil
	}
	if project.Path == "" {
		log.Warn("Skipping clone due to missing path")
		return nil
	}

	for _, repo := range project.Repos {
		err := cloneRepo(project, repo, deps)
		if err != nil {
			errList = append(errList, err)
		}
	}
	for _, subProject := range project.SubProjects {
		fmt.Println()
		errs := cloneProjectRepos(subProject, deps)
		errList = append(errList, errs...)
	}
	return errList
}

func cloneRepo(project domain.Project, repo domain.Repository, deps types.RuntimeCLI) error {
	maxLen := cli.GetMaxLen(project)
	repoTitle := cli.RepoTitle(project, repo, deps.HomeDir, deps.Theme).
		Width(maxLen).
		Render()

	fmt.Printf("%s ", repoTitle)
	defer fmt.Println()

	if repo.State == domain.RepoStateError {
		return cli.AbortOnRepoState(repo, deps.Theme.Error)
	}
	if _, err := os.Stat(repo.AbsPath); !os.IsNotExist(err) {
		repoPath := cli.Path(repo.AbsPath, deps.HomeDir)
		return types.NewWarning("already cloned at %s", repoPath)
	}

	out, err := deps.Git.Clone(repo.Src, repo.AbsPath)
	fmt.Print(deps.Theme.GitOutput.Render(out))
	if err != nil {
		fmt.Print(deps.Theme.Error.Render(err.Error()))
		return cli.RepoError(err, repo)
	}
	return nil
}
