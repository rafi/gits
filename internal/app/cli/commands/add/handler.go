package add

import (
	"fmt"
	"path/filepath"

	"github.com/goccy/go-yaml/ast"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/app"
	pick "github.com/rafi/gits/internal/app/cli/interaction/select"
	"github.com/rafi/gits/internal/format"
	"github.com/rafi/gits/internal/infra/providers"
)

// ExecAdd records one or more already-cloned repositories under a project's
// `repos:` in the config file. It is for a project that lists its
// repositories by hand: a project discovered from a Provider Source already
// knows its repositories, and `gits orphan` is the command that asks what
// such a project's directory holds that the source did not report.
//
// Args: (optional)
//   - project name; created when it does not exist
//   - repositories: each a directory, a glob pattern such as `backend*`, or
//     a clone URL, which is cloned into the current directory first. With
//     none, the current directory is the repository.
func ExecAdd(args []string, deps app.RuntimeCLI) error {
	config, err := load(deps.ConfigPath)
	if err != nil {
		return err
	}

	project, projNode, err := ensureProject(args, config, deps)
	if err != nil {
		return err
	}

	var targets []string
	if len(args) > 1 {
		targets = args[1:]
	}
	paths, err := resolveTargets(targets, deps)
	if err != nil {
		return err
	}

	known := make(map[string]bool, len(project.Repos))
	for _, r := range project.Repos {
		known[filepath.Clean(r.AbsPath)] = true
	}

	added := make([]string, 0, len(paths))
	for _, path := range paths {
		if known[path] {
			// Naming a repository the project already lists is a no-op, not
			// a failure: a glob is expected to sweep up a few of those.
			fmt.Fprintf(deps.Err, "%s is already in project %q, skipping\n",
				format.Path(path, deps.HomeDir), project.Name)
			continue
		}
		remoteURL, err := deps.Git.Remote(deps.Ctx, path)
		if err != nil {
			return fmt.Errorf("unable to read the remote of %s: %w", path, err)
		}
		nicePath := format.Path(path, deps.HomeDir)
		if err := config.addRepo(projNode, nicePath, remoteURL); err != nil {
			return err
		}
		added = append(added, nicePath)
		known[path] = true
	}

	if len(added) == 0 {
		return domain.NewWarning("nothing to add to project %q", project.Name)
	}
	if err := config.save(deps.ConfigPath); err != nil {
		return err
	}

	for _, path := range added {
		fmt.Fprintf(deps.Out, "Added %q repository to project %q\n", path, project.Name)
	}
	return nil
}

// ensureProject returns a project by name along with its mapping in the
// config file, and creates both when the project doesn't exist. If no project
// name is provided, user will be prompted to select one.
func ensureProject(
	args []string, config *configDoc, deps app.RuntimeCLI,
) (domain.Project, *ast.MappingNode, error) {
	if len(args) > 0 {
		if _, foundProject := deps.Projects[args[0]]; !foundProject {
			// Create the project if it doesn't exist.
			project := domain.Project{
				Name:  args[0],
				Repos: []domain.Repository{},
			}
			node, err := config.addProject(project.Name)
			return project, node, err
		}
		// Only the project name is relevant for selection.
		args = args[:1]
	}

	// Get the project we'll be adding to.
	project, _, err := pick.ParseArgs(args, true, deps)
	if err != nil {
		return project, nil, err
	}
	if err := rejectDiscovered(project); err != nil {
		return project, nil, err
	}
	node, err := config.findProject(project.Name)
	return project, node, err
}

// rejectDiscovered refuses a project whose repositories come from a Provider
// Source. Its `repos:` are discovered, not written, so there is nothing for
// `add` to record; the loader gives a project with a path and no `repos:` a
// filesystem source implicitly, so that case gets a hint rather than the
// name of a source the user never wrote.
func rejectDiscovered(project domain.Project) error {
	if project.Source == nil {
		return nil
	}
	if project.Source.Type == string(providers.ProviderFilesystem) {
		return fmt.Errorf(
			"project %q discovers its repositories from %s, so there is nothing to add: "+
				"`gits orphan %s` lists what it holds that no project declares",
			project.Name, project.Path, project.Name)
	}
	return fmt.Errorf(
		"project %q discovers its repositories from %s, so there is nothing to add; "+
			"`gits add` records repositories on a project that lists them by hand",
		project.Name, project.Source.Type)
}
