package add

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cli"
	"github.com/rafi/gits/internal/types"
)

// ExecAdd adds the current repository to a project in the config file.
//
// Args: (optional)
//   - project name
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

	cwd, err := ensureRepository(args, deps)
	if err != nil {
		return err
	}

	remoteURL, err := deps.Git.Remote(deps.Ctx, cwd)
	if err != nil {
		return fmt.Errorf("failed adding repo: %w", err)
	}

	for _, r := range project.Repos {
		if r.AbsPath == cwd {
			return fmt.Errorf("repository already in project %q", project.Name)
		}
	}

	nicePath := cli.Path(cwd, deps.HomeDir)
	appendRepo(nicePath, remoteURL, reposNode)

	if err := save(deps.ConfigPath, rootNode); err != nil {
		return err
	}

	fmt.Fprintf(deps.Out, "Added %q repository to project %q\n", nicePath, project.Name)
	return nil
}

// ensureRepository returns the current repository path, and clones it if it
// doesn't exist.
func ensureRepository(args []string, deps types.RuntimeCLI) (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("unable to get current directory: %w", err)
	}

	if len(args) > 1 {
		// Clone repository if address has been provided.
		remoteURL := args[1]
		cwd = filepath.Join(cwd, domain.RepoDirName(remoteURL))
		output, err := deps.Git.Clone(deps.Ctx, remoteURL, cwd)
		if err != nil {
			// git's own account of the failure, which explains the error
			// returned below rather than being anything the command was
			// asked for.
			fmt.Fprintln(deps.Err, output)
			return "", err
		}
	}

	if !deps.Git.IsRepo(deps.Ctx, cwd) {
		return "", fmt.Errorf("not a git repository: %s", cwd)
	}
	return cwd, nil
}

// ensureProject returns project by name, and creates it if it doesn't exist.
// If no project name is provided, user will be prompted to select one.
func ensureProject(args []string, node *yaml.Node, deps types.RuntimeCLI) (domain.Project, error) {
	if len(args) > 0 {
		if _, foundProject := deps.Projects[args[0]]; !foundProject {
			// Create the project if it doesn't exist.
			project := domain.Project{
				Name:  args[0],
				Repos: []domain.Repository{},
			}
			appendProject(project.Name, node.Content[0])
			return project, nil
		}
		// Only the project name is relevant for selection.
		args = args[:1]
	}

	// Get the project we'll be adding to.
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
