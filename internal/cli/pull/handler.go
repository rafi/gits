package pull

import (
	"fmt"
	"sync"

	"github.com/charmbracelet/lipgloss"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cli"
	"github.com/rafi/gits/internal/types"
)

// ExecPull runs pull --ff-only on project repositories, or on a specific repo.
//
// Args: (optional)
//   - project name
//   - repo
func ExecPull(args []string, deps types.RuntimeCLI) error {
	project, repo, err := cli.ParseArgs(args, true, deps)
	if err != nil {
		return err
	}

	if repo != nil {
		// Pull a single repository.
		resp := pullRepo(project, *repo, deps)
		fmt.Println(resp)
		return resp.error
	}

	// Pull all project's repositories.
	errs := pullProjectRepos(project, deps)
	if len(errs) > 0 {
		return cli.RenderErrors(errs, true)
	}
	return nil
}

type PullResponse struct {
	currentBranch string
	upstream      string
	output        string
	title         lipgloss.Style
	error         error
	errorStyle    lipgloss.Style
}

func (r PullResponse) String() string {
	if r.error != nil {
		return fmt.Sprintf("%s %s", r.title, r.errorStyle.Render(r.error.Error()))
	}

	return fmt.Sprintf(
		"%s [%s <- %s] %s",
		r.title.Render(),
		r.currentBranch,
		r.upstream,
		r.output,
	)
}

func pullProjectRepos(project domain.Project, deps types.RuntimeCLI) []error {
	fmt.Println(cli.ProjectTitleWithBullet(project, deps.Theme))

	errList := make([]error, 0)
	maxLen := cli.GetMaxLen(project)

	var wg sync.WaitGroup
	for idx, repo := range project.Repos {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp := pullRepo(project, repo, deps)
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
		errs := pullProjectRepos(subProject, deps)
		errList = append(errList, errs...)
	}
	return errList
}

func pullRepo(project domain.Project, repo domain.Repository, deps types.RuntimeCLI) PullResponse {
	resp := PullResponse{
		title:      cli.RepoTitle(repo, project.AbsPath, deps.HomeDir, deps.Theme),
		errorStyle: deps.Theme.Error,
	}

	// Abort if repository is not cloned or has errors.
	if repo.State != domain.RepoStateOK {
		resp.error = cli.AbortOnRepoState(repo, deps.Theme.Error)
		return resp
	}

	gitRepo, err := deps.Git.Open(repo.AbsPath)
	if err != nil {
		resp.error = cli.RepoError(err, repo)
		return resp
	}

	resp.currentBranch, err = gitRepo.CurrentBranch()
	if err != nil {
		resp.error = cli.RepoError(err, repo)
		return resp
	}

	upstream, err := gitRepo.GetUpstream(resp.currentBranch)
	if err != nil {
		resp.error = cli.RepoError(err, repo)
		return resp
	}
	resp.upstream = upstream.Short()

	resp.output, err = deps.Git.Pull(repo.AbsPath)
	if err != nil {
		resp.error = cli.RepoError(err, repo)
		return resp
	}
	resp.output = deps.Theme.GitOutput.Render(resp.output)
	return resp
}
