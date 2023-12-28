package clone

import (
	"fmt"
	"os"
	"strings"

	log "github.com/sirupsen/logrus"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cli"
	"github.com/rafi/gits/internal/cli/types"
)

// ExecClone clones project repositories, or a specific repo.
//
// Args: (optional)
//   - project name
//   - repo or sub-project name
func ExecClone(args []string, deps types.RuntimeDeps) error {
	project, err := cli.GetOrSelectProject(args, deps)
	if err != nil {
		return err
	}

	if len(args) > 1 && strings.Index(args[1], "/") > 0 {
		args = args[:len(args)-1]
	}

	// Get specific repo if provided/selected, or all repos in project.
	repos, err := cli.GetOrSelectRepos(project, args, deps)
	if err != nil {
		return err
	}

	if repos == nil {
		var errList []cli.Error
		cloneProjectRepos(project, deps, &errList)
		cli.HandlerErrors(errList)
		return nil
	}

	output, err := cloneRepo(project, repos[0], deps)
	fmt.Println(deps.Theme.GitOutput.Render(output))
	return err
}

func cloneProjectRepos(project domain.Project, deps types.RuntimeDeps, errList *[]cli.Error) {
	errorStyle := deps.Theme.Error.Copy()

	fmt.Println(cli.ProjectTitleWithBullet(project, deps.Theme))

	if project.Clone != nil && !*project.Clone {
		log.Warn("Skipping clone due to config")
		return
	}
	if project.Path == "" {
		log.Warn("Skipping clone due to missing path")
		return
	}

	for _, repo := range project.Repos {
		output, err := cloneRepo(project, repo, deps)
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
		cloneProjectRepos(subProject, deps, errList)
	}
}

func cloneRepo(project domain.Project, repo domain.Repository, deps types.RuntimeDeps) (string, error) {
	maxLen := cli.GetMaxLen(project)
	repoTitle := cli.RepoTitle(project, repo, deps.HomeDir).
		Inherit(deps.Theme.RepoTitle).
		MarginLeft(types.LeftMargin).MarginRight(types.RightMargin).
		Width(maxLen).
		Render()

	fmt.Printf("%s ", repoTitle)

	if repo.State == domain.RepoStateError {
		return "", fmt.Errorf("not a git repository")
	}
	if _, err := os.Stat(repo.AbsPath); !os.IsNotExist(err) {
		repoPath := cli.Path(repo.AbsPath, deps.HomeDir)
		return "", fmt.Errorf("already cloned at %s", repoPath)
	}

	return deps.Git.Clone(repo.Src, repo.AbsPath)
}
