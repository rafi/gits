package clone

import (
	"fmt"
	"os"
	"sync"

	"github.com/charmbracelet/lipgloss"
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
		resp := cloneRepo(project, *repo, deps)
		fmt.Println(resp)
		return err
	}

	// Clone all project's repositories.
	errs := cloneProjectRepos(project, deps)
	if len(errs) > 0 {
		return cli.RenderErrors(errs, true)
	}
	return nil
}

type CloneResponse struct {
	output     string
	title      lipgloss.Style
	error      error
	errorStyle lipgloss.Style
}

func (r CloneResponse) String() string {
	if r.error != nil {
		return fmt.Sprintf("%s %s", r.title, r.errorStyle.Render(r.error.Error()))
	}

	return fmt.Sprintf(
		"%s %s",
		r.title.Render(),
		r.output,
	)
}

func cloneProjectRepos(project domain.Project, deps types.RuntimeCLI) []error {
	fmt.Println(cli.ProjectTitleWithBullet(project, deps.Theme))

	errList := make([]error, 0)
	maxLen := cli.GetMaxLen(project)

	if project.Clone != nil && !*project.Clone {
		log.Warn("Skipping clone due to config")
		return nil
	}

	var wg sync.WaitGroup
	for idx, repo := range project.Repos {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp := cloneRepo(project, repo, deps)
			resp.title.Width(maxLen)
			fmt.Println(resp)
			if resp.error != nil {
				errList = append(errList, resp.error)
			}
		}()
		if idx > 0 && idx%deps.Settings.WorkerCount == 0 {
			wg.Wait()
		}
	}
	wg.Wait()

	for _, subProject := range project.SubProjects {
		fmt.Println()
		errs := cloneProjectRepos(subProject, deps)
		errList = append(errList, errs...)
	}
	return errList
}

func cloneRepo(project domain.Project, repo domain.Repository, deps types.RuntimeCLI) CloneResponse {
	resp := CloneResponse{
		title:      cli.RepoTitle(repo, project.AbsPath, deps.HomeDir, deps.Theme),
		errorStyle: deps.Theme.Error,
	}

	if repo.State == domain.RepoStateError {
		resp.error = cli.AbortOnRepoState(repo, deps.Theme.Error)
		return resp
	}
	if _, err := os.Stat(repo.AbsPath); !os.IsNotExist(err) {
		repoPath := cli.Path(repo.AbsPath, deps.HomeDir)
		resp.error = types.NewWarning("already cloned at %s", repoPath)
		return resp
	}

	var err error
	resp.output, err = deps.Git.Clone(repo.Src, repo.AbsPath)
	if err != nil {
		resp.error = cli.RepoError(err, repo)
		return resp
	}
	resp.output = deps.Theme.GitOutput.Render(resp.output)
	return resp
}
