package cli

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cli/types"
	"github.com/rafi/gits/internal/project"
	"github.com/rafi/gits/pkg/fzf"
)

// ParseArgs parses the arguments and returns the project and repo.
func ParseArgs(args []string, skipRepoSelect bool, deps types.RuntimeDeps) (
	domain.Project, *domain.Repository, error,
) {
	proj, err := getOrSelectProject(args, deps)
	if err != nil {
		return proj, nil, err
	}

	switch {
	case !skipRepoSelect:
		repo, err := getOrSelectRepo(proj, args, deps)
		return proj, &repo, err

	case len(args) > 1 && strings.HasSuffix(args[1], "/"):
		repo, err := getOrSelectRepo(proj, args, deps)
		if err != nil {
			return proj, nil, err
		}
		return proj, &repo, err

	case len(args) < 2:
		return proj, nil, err

	default:
		repo, found := proj.GetRepo(args[1], "")
		if repo.Name == "" || !found {
			return proj, nil, fmt.Errorf("unable to load repo %q", args[1])
		}
		return proj, &repo, err
	}
}

// getOrSelectProject returns a project from the first argument, or
// interactively with fzf.
func getOrSelectProject(args []string, deps types.RuntimeDeps) (
	domain.Project, error,
) {
	var err error
	projName := ""
	if len(args) > 0 {
		projName = args[0]
	} else {
		projName, err = SelectProject(deps)
		if projName == "" || err != nil {
			err = fmt.Errorf("unable to select a project: %w", err)
			return domain.Project{}, err
		}
	}

	// Find project by name.
	p, err := project.GetProject(projName, deps)
	if err != nil {
		return p, fmt.Errorf("unable to load project %q: %w", projName, err)
	}

	// Find a sub-project if provided via 2nd argument.
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
	deps types.RuntimeDeps,
) (domain.Repository, error) {
	var err error
	rootProject := ""
	repoName := ""
	if len(args) > 1 {
		if strings.HasSuffix(args[1], "/") {
			rootProject = args[0]
		} else {
			repoName = args[1]
		}
	}
	if repoName == "" {
		repoName, err = SelectRepo(rootProject, project, deps)
		if repoName == "" || err != nil {
			err = fmt.Errorf("unable to select a repo: %w", err)
			return domain.Repository{}, err
		}
	}

	repo, found := project.GetRepo(repoName, "")
	if repo.Name == "" || !found {
		return repo, fmt.Errorf("unable to load repo %q", repoName)
	}
	return repo, nil
}

// SelectProject returns an interactively selected project name.
func SelectProject(deps types.RuntimeDeps) (string, error) {
	// Collect project names
	buffer := bytes.Buffer{}
	for name, project := range deps.Projects {
		project.Name = name
		projectTitle := ProjectTitle(project, deps.Theme)
		buffer.WriteString(projectTitle + "\n")
	}

	// Run fzf with the sub-command 'list' as preview.
	finder := fzf.New("--nth=1")
	finder.WithPrompt("project> ")

	previewCmd := "gits -C=always --config='%s' list -o tree {1}"
	previewCmd = fmt.Sprintf(previewCmd, deps.Source)
	finder.WithPreview(previewCmd, "")

	projName, err := finder.Run(buffer)
	if projName == "" || err != nil {
		err = fmt.Errorf("unable to select a project: %w", err)
		return "", err
	}
	projName = strings.Split(projName, " ")[0]
	return projName, nil
}

// SelectRepo returns an interactively selected repository name.
func SelectRepo(
	rootProject string,
	project domain.Project,
	deps types.RuntimeDeps,
) (string, error) {
	// Collect repo names
	style := deps.Theme.RepoTitle
	buffer := bytes.Buffer{}
	repos := project.ListReposWithNamespace()
	for _, repo := range repos {
		buffer.WriteString(style.Render(repo) + "\n")
	}

	// rootProject is empty when a root project is provided.
	prefix := project.Name
	if rootProject == "" {
		prefix = ""
		rootProject = project.Name
	}

	// Run fzf with the hidden sub-command 'repo-overview' as preview.
	finder := fzf.New()
	finder.WithPrompt(fmt.Sprintf("[%s] repo> ", project.Name))

	previewCmd := "gits -C=always --config='%s' repo-overview '%s' '%s'{}"
	previewCmd = fmt.Sprintf(previewCmd, deps.Source, rootProject, prefix)
	finder.WithPreview(previewCmd, "")

	repoName, err := finder.Run(buffer)
	if repoName == "" || err != nil {
		err = fmt.Errorf("unable to select a repository: %w", err)
		return "", err
	}
	return repoName, nil
}

// SelectBranch returns an interactively selected branch name.
func SelectBranch(
	projName string,
	repo domain.Repository,
	deps types.RuntimeDeps,
) (string, error) {
	refs, err := deps.Git.Refs(repo.AbsPath)
	if err != nil {
		return "", fmt.Errorf("unable to open repo: %w", err)
	}

	delimiter := "\t"

	branchLabel := deps.Theme.BranchIndicator.Render("branch") + delimiter
	tagLabel := deps.Theme.TagIndicator.Render("tag") + delimiter

	buffer := bytes.Buffer{}
	for _, ref := range refs {
		ref = strings.Replace(ref, "refs/tags/", tagLabel, 1)
		ref = strings.Replace(ref, "refs/heads/", branchLabel, 1)
		buffer.WriteString(ref + "\n")
	}

	repoFullName := repo.GetNameWithNamespace()

	// Run fzf with the hidden sub-command 'branch-overview' as preview.
	finder := fzf.New("--delimiter="+delimiter, "--nth=2")
	finder.WithPrompt(fmt.Sprintf("[%s/%s] branch> ", projName, repoFullName))

	previewCmd := "gits -C=always --config='%s' branch-overview '%s' '%s' {2}"
	previewCmd = fmt.Sprintf(previewCmd, deps.Source, projName, repoFullName)
	finder.WithPreview(previewCmd, "")

	selected, err := finder.Run(buffer)
	if err != nil {
		return "", err
	}
	branchName := strings.Split(selected, delimiter)[1]
	return branchName, nil
}
