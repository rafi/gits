// Package add implements `gits add`.
package add

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/goccy/go-yaml/ast"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/app"
	pick "github.com/rafi/gits/internal/app/cli/interaction/select"
	"github.com/rafi/gits/internal/config/edit"
	"github.com/rafi/gits/internal/format"
	"github.com/rafi/gits/internal/infra/providers"
)

// ExecAdd records one or more already-cloned repositories under a project's
// `repos:` in the config file. It is for a project that lists its
// repositories by hand: a project discovered from a Provider Source already
// knows its repositories, and `gits orphan` is the command that asks what
// such a project's directory holds that the source did not report.
//
// A `--tag` labels every named repository, including already-listed ones.
//
// Args: (optional)
//   - project name; created when it does not exist
//   - repositories: each a directory, a glob pattern such as `backend*`, or
//     a clone URL, which is cloned into the current directory first. With
//     none, the current directory is the repository.
func ExecAdd(tags domain.TagSet, args []string, deps app.RuntimeCLI) error {
	if len(args) == 0 && len(deps.Projects) == 0 {
		// Without a project name the project is picked interactively, and an
		// empty list gives nothing to pick. Fail before creating a config file.
		return errors.New(
			"no projects are configured, so there is nothing to add to: " +
				"name the project to create, as in `gits add myproject`")
	}

	configPath, created, err := edit.EnsurePath(deps.ConfigPath)
	if err != nil {
		return err
	}
	if created {
		fmt.Fprintf(deps.Err, "Created config file %s\n", format.Path(configPath, deps.HomeDir))
	}
	doc, err := edit.Load(configPath)
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

	// Listed repositories keyed by absolute path.
	known := make(map[string]domain.Repository, len(project.Repos))
	for _, r := range project.Repos {
		known[filepath.Clean(r.AbsPath)] = r
	}

	added, tagged, err := ensureRepo(doc, projNode, paths, known, tags, project, deps)
	if err != nil {
		return err
	}

	if len(added) == 0 && len(tagged) == 0 {
		return domain.NewWarning("nothing to add to project %q", project.Name)
	}
	if err := doc.Save(configPath); err != nil {
		return err
	}

	tagNames := tags.Names()
	for _, path := range added {
		if len(tagNames) > 0 {
			fmt.Fprintf(deps.Out, "Added %q repository to project %q, tagged %s\n",
				path, project.Name, tags)
			continue
		}
		fmt.Fprintf(deps.Out, "Added %q repository to project %q\n", path, project.Name)
	}
	for _, path := range tagged {
		fmt.Fprintf(deps.Out, "Tagged %q in project %q with %s\n", path, project.Name, tags)
	}
	return nil
}

// ensureRepo records each path in the project's `repos:` and returns the
// repositories it added and the listed ones it tagged. It updates known.
func ensureRepo(
	doc *edit.Doc,
	projNode *ast.MappingNode,
	paths []string,
	known map[string]domain.Repository,
	tags domain.TagSet,
	project domain.Project,
	deps app.RuntimeCLI,
) (added, tagged []string, err error) {
	tagNames := tags.Names()
	added = make([]string, 0, len(paths))
	tagged = make([]string, 0, len(paths))
	for _, path := range paths {
		nicePath := format.Path(path, deps.HomeDir)
		if entry, listed := known[path]; listed {
			if len(tagNames) == 0 {
				fmt.Fprintf(deps.Err, "%s is already in project %q, skipping\n",
					nicePath, project.Name)
				continue
			}
			changed, err := doc.TagRepo(projNode, entry.Dir, entry.Src, tagNames)
			switch {
			case errors.Is(err, edit.ErrRepoNotListed):
				fmt.Fprintf(deps.Err,
					"%s is in project %q, but not as an entry of its own `repos:`, not tagged\n",
					nicePath, project.Name)
			case err != nil:
				return nil, nil, err
			case changed:
				tagged = append(tagged, nicePath)
			default:
				fmt.Fprintf(deps.Err, "%s in project %q already carries %s\n",
					nicePath, project.Name, strings.Join(tagNames, ","))
			}
			continue
		}
		remoteURL, err := deps.Git.Remote(deps.Ctx, path)
		if err != nil {
			return nil, nil, fmt.Errorf("unable to read the remote of %s: %w", path, err)
		}
		if err := doc.AddRepo(projNode, nicePath, remoteURL, tagNames...); err != nil {
			return nil, nil, err
		}
		added = append(added, nicePath)
		known[path] = domain.Repository{Dir: nicePath, Src: remoteURL}
	}
	return added, tagged, nil
}

// ensureProject returns a project by name along with its mapping in the
// config file, and creates both when the project doesn't exist. If no project
// name is provided, user will be prompted to select one.
func ensureProject(
	args []string, doc *edit.Doc, deps app.RuntimeCLI,
) (domain.Project, *ast.MappingNode, error) {
	if len(args) > 0 {
		if _, foundProject := deps.Projects[args[0]]; !foundProject {
			// Create the project if it doesn't exist.
			project := domain.Project{
				Name:  args[0],
				Repos: []domain.Repository{},
			}
			node, err := doc.AddProject(project.Name)
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
	node, err := doc.FindProject(project.Name)
	return project, node, err
}

// rejectDiscovered refuses a project whose repositories come from a Provider
// Source. Its `repos:` are discovered, not written, so there is nothing for
// `add` to record.
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
