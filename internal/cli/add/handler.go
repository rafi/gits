package add

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cli"
	"github.com/rafi/gits/internal/types"
)

// ExecAdd adds one or more repositories to a project in the config file.
//
// Args: (optional)
//   - project name
//   - repository paths, a glob pattern, or a remote URL
func ExecAdd(args []string, deps types.RuntimeCLI) error {
	// Load the config file.
	rootNode, err := load(deps.ConfigPath)
	if err != nil {
		return err
	}

	project, err := ensureProject(args, &rootNode, deps)
	if err != nil {
		return err
	}

	projNode, err := findProject(project.Name, &rootNode)
	if err != nil {
		return err
	}
	reposNode, err := findScalarMapping("repos", projNode)
	if err != nil {
		return err
	}

	repositoryPaths, err := ensureRepositories(args, deps)
	if err != nil {
		return err
	}

	existingPaths := make(map[string]struct{}, len(project.Repos))
	for _, repo := range project.Repos {
		existingPaths[filepath.Clean(repo.AbsPath)] = struct{}{}
	}

	addedPaths := make([]string, 0, len(repositoryPaths))
	for _, repositoryPath := range repositoryPaths {
		if _, exists := existingPaths[filepath.Clean(repositoryPath)]; exists {
			continue
		}

		remoteURL, err := deps.Git.Remote(repositoryPath)
		if err != nil {
			return fmt.Errorf("failed adding repository %q: %w", repositoryPath, err)
		}

		nicePath := cli.Path(repositoryPath, deps.HomeDir)
		appendRepo(nicePath, remoteURL, reposNode)
		addedPaths = append(addedPaths, nicePath)
		existingPaths[filepath.Clean(repositoryPath)] = struct{}{}
	}

	if len(addedPaths) == 0 {
		return fmt.Errorf("all matched repositories are already in project %q", project.Name)
	}

	if err := save(deps.ConfigPath, rootNode); err != nil {
		return err
	}

	if len(addedPaths) == 1 {
		fmt.Printf("Added %q repository to project %q\n", addedPaths[0], project.Name)
	} else {
		fmt.Printf("Added %d repositories to project %q\n", len(addedPaths), project.Name)
		for _, path := range addedPaths {
			fmt.Printf("  - %s\n", path)
		}
	}
	return nil
}

// ensureRepositories returns repository paths selected by the arguments. A
// single argument that is neither a local path nor a glob retains the existing
// behavior of being cloned as a remote URL.
func ensureRepositories(args []string, deps types.RuntimeCLI) ([]string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("unable to get current directory: %w", err)
	}

	if len(args) < 2 {
		if !deps.Git.IsRepo(cwd) {
			return nil, fmt.Errorf("not a git repository: %s", cwd)
		}
		return []string{cwd}, nil
	}

	targets := args[1:]
	if len(targets) == 1 && !hasGlobMeta(targets[0]) {
		path := resolvePath(cwd, targets[0])
		info, statErr := os.Stat(path)
		switch {
		case statErr == nil:
			if !info.IsDir() || !deps.Git.IsRepo(path) {
				return nil, fmt.Errorf("not a git repository: %s", path)
			}
			return []string{path}, nil
		case !os.IsNotExist(statErr):
			return nil, fmt.Errorf("unable to inspect repository %q: %w", path, statErr)
		}

		// Clone a single target that does not resolve to a local path.
		remoteURL := targets[0]
		baseName := strings.TrimSuffix(filepath.Base(remoteURL), ".git")
		path = filepath.Join(cwd, baseName)
		output, err := deps.Git.Clone(remoteURL, path)
		if err != nil {
			fmt.Println(output)
			return nil, err
		}
		return []string{path}, nil
	}

	paths, err := expandRepositoryTargets(cwd, targets, deps.Git.IsRepo)
	if err != nil {
		return nil, err
	}
	return paths, nil
}

// ensureProject returns project by name, and creates it if it doesn't exist.
// If no project name is provided, user will be prompted to select one.
func ensureProject(args []string, node *yaml.Node, deps types.RuntimeCLI) (domain.Project, error) {
	foundProject := false
	if len(args) > 0 {
		args = args[0:1]
		for projName := range deps.Projects {
			if projName == args[0] {
				foundProject = true
				break
			}
		}
	}

	if len(args) == 0 || foundProject {
		// Get the project we'll be adding to.
		var err error
		project, _, err := cli.ParseArgs(args, true, deps)
		if err != nil {
			return project, err
		}

		// Disallow cloud projects.
		if project.Source != nil {
			return project, fmt.Errorf(
				"project %q is sourced from %s, choose a regular non-cloud project",
				project.Name,
				project.Source.Type,
			)
		}
		return project, nil
	}

	// Create the project if it doesn't exist.
	project := domain.Project{
		Name:  args[0],
		Repos: []domain.Repository{},
	}
	appendProject(project.Name, node.Content[0])
	return project, nil
}
