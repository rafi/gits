package add

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/goccy/go-yaml/ast"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/app"
	pick "github.com/rafi/gits/internal/app/cli/interaction/select"
	"github.com/rafi/gits/internal/config"
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
	if len(args) == 0 && len(deps.Projects) == 0 {
		// Without a project name the project is picked interactively, and an
		// empty list gives nothing to pick. Fail before creating a config file.
		return errors.New(
			"no projects are configured, so there is nothing to add to: " +
				"name the project to create, as in `gits add myproject`")
	}

	configPath, err := ensureConfigFile(deps)
	if err != nil {
		return err
	}
	doc, err := load(configPath)
	if err != nil {
		return err
	}

	project, projNode, err := ensureProject(args, doc, deps)
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
		if err := doc.addRepo(projNode, nicePath, remoteURL); err != nil {
			return err
		}
		added = append(added, nicePath)
		known[path] = true
	}

	if len(added) == 0 {
		return domain.NewWarning("nothing to add to project %q", project.Name)
	}
	if err := doc.save(configPath); err != nil {
		return err
	}

	for _, path := range added {
		fmt.Fprintf(deps.Out, "Added %q repository to project %q\n", path, project.Name)
	}
	return nil
}

// ensureConfigFile returns the config file `add` should write to, creating an
// empty one for a user who has none. A user with a config file always gets
// that file: the empty ConfigPath this acts on means the loader searched every
// default location and found nothing, and the new file is created with
// O_EXCL, so a file that appeared in the meantime is used as it is rather than
// truncated. Nothing here ever writes over a config that already exists.
func ensureConfigFile(deps app.RuntimeCLI) (string, error) {
	if deps.ConfigPath != "" {
		return deps.ConfigPath, nil
	}
	// Re-run the loader's own search: ConfigPath is empty both for a user with
	// no config file and for a test that left the field unset.
	existing, err := config.ExistingPath()
	if err != nil {
		return "", err
	}
	if existing != "" {
		return existing, nil
	}

	path, err := config.NewFilePath()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", fmt.Errorf("unable to create config directory: %w", err)
	}
	// A config file can hold provider tokens, so a file gits creates is the
	// user's own to read; one that already exists keeps the mode they chose.
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
	switch {
	case os.IsExist(err):
		// Another process wrote one between the search and now: append to it.
		return path, nil
	case err != nil:
		return "", fmt.Errorf("unable to create config file: %w", err)
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("unable to create config file: %w", err)
	}
	fmt.Fprintf(deps.Err, "Created config file %s\n", format.Path(path, deps.HomeDir))
	return path, nil
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
