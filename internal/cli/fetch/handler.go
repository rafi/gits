package fetch

import (
	"fmt"
	"sync"

	"github.com/charmbracelet/lipgloss"

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
		resp := fetchRepo(project, *repo, deps)
		fmt.Println(resp)
		return err
	}

	// Fetch all project's repositories.
	errs := fetchProjectRepos(project, deps)
	if len(errs) > 0 {
		return cli.RenderErrors(errs, true)
	}
	return nil
}

type FetchResponse struct {
	repoPath   string
	output     string
	title      lipgloss.Style
	error      error
	errorStyle lipgloss.Style
}

func (r FetchResponse) String() string {
	if r.error != nil {
		return fmt.Sprintf("%s %s", r.title, r.errorStyle.Render(r.error.Error()))
	}

	if r.title.Value() == r.repoPath {
		return fmt.Sprintf("%s %s", r.title, r.output)
	}
	return fmt.Sprintf("%s %s %s", r.title, r.repoPath, r.output)
}

func fetchProjectRepos(project domain.Project, deps types.RuntimeCLI) []error {
	fmt.Println(cli.ProjectTitleWithBullet(project, deps.Theme))

	errList := make([]error, 0)
	maxLen := cli.GetMaxLen(project)

	var wg sync.WaitGroup
	for idx, repo := range project.Repos {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp := fetchRepo(project, repo, deps)
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
		errs := fetchProjectRepos(subProject, deps)
		errList = append(errList, errs...)
	}
	return errList
}

func fetchRepo(project domain.Project, repo domain.Repository, deps types.RuntimeCLI) FetchResponse {
	resp := FetchResponse{
		title:      cli.RepoTitle(repo, project.AbsPath, deps.HomeDir, deps.Theme),
		repoPath:   cli.Path(repo.AbsPath, deps.HomeDir),
		errorStyle: deps.Theme.Error,
	}

	// Abort if repository is not cloned or has errors.
	if repo.State != domain.RepoStateOK {
		resp.error = cli.AbortOnRepoState(repo, deps.Theme.Error)
		return resp
	}

	var err error
	resp.output, err = deps.Git.Fetch(repo.AbsPath)
	if err != nil {
		resp.error = cli.RepoError(err, repo)
		return resp
	}
	resp.output = deps.Theme.GitOutput.Render(resp.output)
	return resp
}
