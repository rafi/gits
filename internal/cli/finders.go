// Package cli holds what every gits command shares: Project and Repository
// selection, title rendering, path formatting, and the error epilogue.
package cli

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/fzf"
	"github.com/rafi/gits/internal/loader"
	"github.com/rafi/gits/internal/types"
)

// branchLineFields is how many tab-separated fields a branch selection line
// carries: the kind indicator and the ref name.
const branchLineFields = 2

// isCancelled reports whether an interactive selection ended without a
// choice — the user pressed Esc/Ctrl-C or there was nothing to match.
func isCancelled(err error) bool {
	return errors.Is(err, fzf.ErrAborted) || errors.Is(err, fzf.ErrNoMatch)
}

// ParseArgs parses the arguments and returns the project and repo.
func ParseArgs(args []string, skipRepoSelect bool, deps types.RuntimeCLI) (
	domain.Project, *domain.Repository, error,
) {
	proj, err := getOrSelectProject(args, deps)
	if err != nil {
		return proj, nil, err
	}

	// Select a repo when the command demands one, or when a 2nd argument
	// names one directly (a trailing "/" means sub-project, not repo).
	if !skipRepoSelect || (len(args) > 1 && !strings.HasSuffix(args[1], "/")) {
		repo, err := getOrSelectRepo(proj, args, deps)
		if err != nil {
			return proj, nil, err
		}
		return proj, &repo, nil
	}
	return proj, nil, nil
}

// getOrSelectProject returns a project from the first argument, or
// interactively with fzf.
func getOrSelectProject(args []string, deps types.RuntimeCLI) (
	domain.Project, error,
) {
	var (
		err      error
		projName string
	)
	if len(args) > 0 {
		projName = args[0]
	} else {
		projName, err = SelectProject(deps)
		if err != nil {
			return domain.Project{}, err
		}
	}
	if projName == "" {
		return domain.Project{}, types.NewWarning("no project selected")
	}

	// Find project by name. Naming a project that does not exist is a real
	// failure, not a downgradeable warning: a script must be able to tell a
	// typo from success.
	p, err := loader.GetProject(projName, deps.Runtime)
	if err != nil {
		return p, fmt.Errorf("unable to load project: %w", err)
	}

	// Find a sub-project if provided via 2nd argument. A named sub-project
	// that does not exist is likewise a failure.
	if len(args) > 1 && strings.HasSuffix(args[1], "/") {
		var found bool
		p, found = p.GetSubProject(args[1], "")
		if !found {
			return p, fmt.Errorf("project %q not found", args[1])
		}
		p.Name = args[1]
	}
	return p, nil
}

// getOrSelectRepo returns a repository from the 2nd argument, or
// interactively with fzf.
func getOrSelectRepo(
	project domain.Project,
	args []string,
	deps types.RuntimeCLI,
) (domain.Repository, error) {
	var err error
	rootProject := ""
	repoName := ""
	if len(args) > 1 {
		if strings.HasSuffix(args[1], "/") {
			// The preview re-invokes gits with the root project, which for a
			// path argument is the name the loader derived from it.
			rootProject = loader.ProjectName(args[0])
		} else {
			repoName = args[1]
		}
	}
	if repoName == "" {
		repoName, err = SelectRepo(rootProject, project, deps)
		if err != nil {
			return domain.Repository{}, err
		}
		if repoName == "" {
			return domain.Repository{}, types.NewWarning("no repository selected")
		}
	}

	repo, found := project.GetRepo(repoName, "")
	if !found {
		return repo, fmt.Errorf("repo %q not found", repoName)
	}
	return repo, nil
}

// SelectProject returns an interactively selected project name.
func SelectProject(deps types.RuntimeCLI) (string, error) {
	// Collect project names in a stable order so the picker does not reshuffle
	// between runs.
	buffer := bytes.Buffer{}
	for _, name := range deps.Projects.SortedNames() {
		project := deps.Projects[name]
		project.Name = name
		projectTitle := ProjectTitle(project, deps.Theme)
		buffer.WriteString(projectTitle)
		buffer.WriteByte('\n')
	}

	// Run fzf with the sub-command 'list' as preview.
	finder := fzf.New(deps.Err, "--nth=1").WithFinder(deps.Settings.Finder)
	finder.WithPrompt("project> ")

	previewCmd := previewCommandf(deps.ConfigPath, "list", "{1}", "-o", "tree")
	finder.WithPreview(previewCmd, "")

	projName, err := finder.Run(deps.Ctx, buffer)
	if err != nil {
		if isCancelled(err) {
			return "", nil
		}
		return "", err
	}
	projName = strings.Split(projName, " ")[0]
	return projName, nil
}

// SelectRepo returns an interactively selected repository name.
func SelectRepo(
	rootProject string,
	project domain.Project,
	deps types.RuntimeCLI,
) (string, error) {
	// Collect repo names
	style := deps.Theme.RepoTitle
	buffer := bytes.Buffer{}
	repos := project.ListReposWithNamespace()
	for _, repo := range repos {
		buffer.WriteString(style.Render(repo))
		buffer.WriteByte('\n')
	}

	// rootProject is empty when a root project is provided.
	prefix := project.Name
	if rootProject == "" {
		prefix = ""
		rootProject = project.Name
	}

	// Run fzf with the hidden sub-command 'repo-overview' as preview.
	finder := fzf.New(deps.Err).WithFinder(deps.Settings.Finder)
	finder.WithPrompt(fmt.Sprintf("[%s] repo> ", project.Name))

	// {} carries the selected line and is appended to the quoted prefix with
	// no space: the two together are one argument.
	previewCmd := previewCommand(deps.ConfigPath, "repo-overview", rootProject, prefix) + "{}"
	finder.WithPreview(previewCmd, "")

	repoName, err := finder.Run(deps.Ctx, buffer)
	if err != nil {
		if isCancelled(err) {
			return "", nil
		}
		return "", fmt.Errorf("unable to select a repository: %w", err)
	}
	return repoName, nil
}

// SelectBranch returns an interactively selected branch name.
func SelectBranch(
	projName string,
	repo domain.Repository,
	deps types.RuntimeCLI,
) (string, error) {
	refs, err := deps.Git.Refs(deps.Ctx, repo.AbsPath)
	if err != nil {
		return "", fmt.Errorf("unable to list branches and tags: %w", err)
	}

	delimiter := "\t"

	branchLabel := deps.Theme.BranchIndicator.Render("branch") + delimiter
	tagLabel := deps.Theme.TagIndicator.Render("tag") + delimiter

	buffer := bytes.Buffer{}
	for _, ref := range refs {
		ref = strings.Replace(ref, "refs/tags/", tagLabel, 1)
		ref = strings.Replace(ref, "refs/heads/", branchLabel, 1)
		buffer.WriteString(ref)
		buffer.WriteByte('\n')
	}

	repoFullName := repo.GetNameWithNamespace()

	// Run fzf with the hidden sub-command 'branch-overview' as preview.
	finder := fzf.New(deps.Err, "--delimiter="+delimiter, "--nth=2").
		WithFinder(deps.Settings.Finder)
	finder.WithPrompt(fmt.Sprintf("[%s/%s] branch> ", projName, repoFullName))

	previewCmd := previewCommandf(
		deps.ConfigPath, "branch-overview", "{2}", projName, repoFullName)
	finder.WithPreview(previewCmd, "")

	selected, err := finder.Run(deps.Ctx, buffer)
	if err != nil {
		if isCancelled(err) {
			return "", types.NewWarning("no branch selected")
		}
		return "", err
	}
	parts := strings.SplitN(selected, delimiter, 3)
	if len(parts) < branchLineFields {
		return "", types.NewWarning("no branch selected")
	}
	return parts[1], nil
}
