package cli

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cli/types"
	"github.com/rafi/gits/internal/fzf"
	"github.com/rafi/gits/internal/project"
)

// GetOrSelectProject returns a project from the first argument, or
// interactively with fzf.
func GetOrSelectProject(args []string, deps types.RuntimeDeps) (domain.Project, error) {
	var err error
	projName := ""
	if len(args) > 0 {
		projName = args[0]
	} else {
		projName, err = SelectProject(deps)
		if projName == "" || err != nil {
			err = fmt.Errorf("unable to interactively select a project: %w", err)
			return domain.Project{}, err
		}
	}

	// Find project by name.
	p, err := project.GetProject(projName, deps)
	if err != nil {
		return p, fmt.Errorf("unable to load project %q: %w", projName, err)
	}

	// Find a sub-project if provided via 2nd argument.
	if len(args) > 1 && strings.Index(args[1], "/") > 0 {
		var found bool
		p, found = p.GetSubProject(args[1], "")
		if !found {
			return p, fmt.Errorf("project %q not found", args[1])
		}
		p.Name = args[1]
	}
	return p, nil
}

// GetOrSelectRepos logic:
//   - If no args are provided, select a project and repo.
//   - If only project name is provided, use all repos in the project.
//   - If project and repo name are provided, use only that repo.
func GetOrSelectRepos(project domain.Project, args []string, deps types.RuntimeDeps) ([]domain.Repository, error) {
	// Use all repos by default, or specific repo if provided via args.
	repos := []domain.Repository{}
	switch len(args) {
	case 2:
		repo, found := project.GetRepo(args[1], "")
		if repo.Name == "" || !found {
			return nil, fmt.Errorf("repo %q not found", args[1])
		}
		return repos, nil

	case 1:
		return nil, nil

	case 0:
		repo, _, err := GetOrSelectRepo(project, args, deps)
		if err != nil {
			err = fmt.Errorf("unable to select repo: %w", err)
			return nil, err
		}
		return []domain.Repository{repo}, nil
	}

	return nil, fmt.Errorf("invalid arguments: %v", args)
}

// GetOrSelectRepo returns a repository from the 2nd argument, or interactively.
func GetOrSelectRepo(project domain.Project, args []string, deps types.RuntimeDeps) (domain.Repository, string, error) {
	var err error
	repoName := ""
	if len(args) > 1 {
		repoName = args[1]
	} else {
		repoName, err = SelectRepo(project, deps)
		if repoName == "" || err != nil {
			err = fmt.Errorf("unable to interactively select a repo: %w", err)
			return domain.Repository{}, "", err
		}
	}

	repo, found := project.GetRepo(repoName, "")
	if repo.Name == "" || !found {
		return repo, "", fmt.Errorf("unable to load repo %q", repoName)
	}
	return repo, repoName, nil
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

	// Run fzf with preview to 'list' sub-command.
	previewCommand := fmt.Sprintf(
		"--preview=gits --config='%s' --color=always list -o tree {1}",
		deps.Source,
	)
	projName, err := fzf.Run(
		buffer,
		"--prompt=project> ",
		"--nth=1",
		previewCommand,
		"--preview-window=right,70%",
	)
	if err != nil {
		return "", err
	}
	projName = strings.Split(projName, " ")[0]
	return projName, nil
}

// SelectRepo returns an interactively selected repository name.
func SelectRepo(project domain.Project, deps types.RuntimeDeps) (string, error) {
	// Collect repo names
	style := deps.Theme.RepoTitle
	buffer := bytes.Buffer{}
	repos := project.ListReposWithNamespace()
	for _, repo := range repos {
		buffer.WriteString(style.Render(repo) + "\n")
	}

	// Run fzf with preview to hidden 'repo-overview' sub-command.
	previewCommand := fmt.Sprintf(
		"--preview=gits --config='%s' --color=always repo-overview '%s' {}",
		deps.Source,
		project.Name,
	)
	repoName, err := fzf.Run(
		buffer,
		fmt.Sprintf("--prompt=[%s] repo> ", project.Name),
		previewCommand,
		"--preview-window=right,70%",
	)
	if err != nil {
		return "", err
	}
	return repoName, nil
}

// SelectBranch returns an interactively selected branch name.
func SelectBranch(projName, repoFullName string, repo domain.Repository, deps types.RuntimeDeps) (string, error) {
	refs, err := deps.Git.Refs(repo.AbsPath)
	if err != nil {
		return "", fmt.Errorf("unable to open repo: %w", err)
	}

	branchStyle := deps.Theme.BranchIndicator
	tagStyle := deps.Theme.TagIndicator

	buffer := bytes.Buffer{}
	for _, ref := range refs {
		ref = strings.Replace(ref, "refs/tags/", tagStyle.Render("tag")+"\t", 1)
		ref = strings.Replace(ref, "refs/heads/", branchStyle.Render("branch")+"\t", 1)
		buffer.WriteString(ref + "\n")
	}

	previewCommand := fmt.Sprintf(
		"--preview=gits --config='%s' --color=always branch-overview '%s' '%s' {2}",
		deps.Source,
		projName,
		repoFullName,
	)
	branch, err := fzf.Run(
		buffer,
		fmt.Sprintf("--prompt=[%s/%s] branch> ", projName, repoFullName),
		"--delimiter=\t",
		"--nth=2",
		previewCommand,
		"--preview-window=right,70%",
	)
	if err != nil {
		return "", err
	}
	branch = strings.Split(branch, "\t")[1]
	return branch, nil
}
