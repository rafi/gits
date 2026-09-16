// Package discover implements `gits discover`, which records directories of
// git repositories as Projects in the config file.
package discover

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/app"
	"github.com/rafi/gits/internal/config/edit"
	"github.com/rafi/gits/internal/format"
	"github.com/rafi/gits/internal/service/discover"
)

// ErrNoPath is returned when no path to scan is given.
var ErrNoPath = errors.New(
	"name the path to scan: `gits discover ~/src`, or `gits discover .` for the current directory")

// DefaultMin is the default for Options.Min.
const DefaultMin = discover.DefaultMin

// Options are the command's own flags.
type Options struct {
	// Min is the fewest repositories a directory needs to become a Project.
	Min int
	// DryRun reports what would be written and writes nothing.
	DryRun bool
}

// ExecDiscover records each directory of git repositories under a path as a
// Project, appending new clones to Projects already configured.
//
// Args:
//   - path to scan (required)
func ExecDiscover(opts Options, args []string, deps app.RuntimeCLI) error {
	if len(args) == 0 {
		// Normally refused earlier by the command's Args.
		return ErrNoPath
	}
	root := args[0]

	result, err := discover.Scan(root, deps.Runtime, discover.Options{
		Min:      opts.Min,
		Existing: deps.Projects,
	})
	if err != nil {
		return err
	}

	writable := result.Writable()
	if len(writable) == 0 {
		reportTaken(result, deps)
		return domain.NewWarning("%s", nothingFound(result, root, opts.Min))
	}

	// Show every group before writing anything.
	for _, group := range writable {
		lipgloss.Fprintln(deps.Out, renderGroup(group, deps))
	}
	reportTaken(result, deps)

	if opts.DryRun {
		lipgloss.Fprintln(deps.Err, deps.Theme.StatusFooter.Render(
			fmt.Sprintf("Would add %d %s. Nothing was written.",
				len(writable), plural(len(writable), "project", "projects"))))
		return nil
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

	added := 0
	for _, group := range writable {
		// Append new clones to the project configured at this path.
		if group.Existing != "" {
			n, err := appendToProject(doc, group, configPath, deps)
			if err != nil {
				return err
			}
			added += n
			continue
		}
		// Also check the file: a name the loader skipped is still taken.
		if doc.HasProject(group.Name) {
			fmt.Fprintf(deps.Err,
				"a project named %q is already in the config file, skipping %s\n",
				group.Name, format.Path(group.Path, deps.HomeDir))
			continue
		}
		if err := doc.AddProjectRepos(
			group.Name, format.Path(group.Path, deps.HomeDir), repoEntries(group, deps),
		); err != nil {
			return err
		}
		added++
	}
	if added == 0 {
		return domain.NewWarning("every project found is already in %s",
			format.Path(configPath, deps.HomeDir))
	}
	if err := doc.Save(configPath); err != nil {
		return err
	}

	lipgloss.Fprintln(deps.Err, deps.Theme.StatusFooter.Render(
		fmt.Sprintf("Added %d %s to %s.", added, plural(added, "project", "projects"),
			format.Path(configPath, deps.HomeDir))))
	return nil
}

// appendToProject adds a group's repositories to its configured project and
// returns how many projects changed. A project not in the file is reported.
func appendToProject(
	doc *edit.Doc, group discover.Group, configPath string, deps app.RuntimeCLI,
) (int, error) {
	node := doc.SubProject(group.ExistingNames)
	if node == nil {
		fmt.Fprintf(deps.Err,
			"%s belongs to project %q, which is not in %s, skipping\n",
			format.Path(group.Path, deps.HomeDir), group.Existing,
			format.Path(configPath, deps.HomeDir))
		return 0, nil
	}
	for _, repo := range repoEntries(group, deps) {
		if err := doc.AddRepo(node, repo.Dir, repo.Src); err != nil {
			return 0, err
		}
	}
	fmt.Fprintf(deps.Err, "Added %d %s to the existing project %q\n",
		len(group.Repos), plural(len(group.Repos), "repository", "repositories"),
		group.Existing)
	return 1, nil
}

// repoEntries returns config entries for a group's repositories, with dirs
// relative to the project they are written under.
func repoEntries(group discover.Group, deps app.RuntimeCLI) []edit.Repo {
	base := group.Path
	if group.Existing != "" {
		// Relative to the existing project's path, or in full without one.
		base = group.ExistingPath
	}
	out := make([]edit.Repo, 0, len(group.Repos))
	for _, repo := range group.Repos {
		out = append(out, edit.Repo{Dir: repoDir(base, repo.Path, deps), Src: repo.Src})
	}
	return out
}

// repoDir returns path relative to base, or in full when it is outside base.
func repoDir(base, path string, deps app.RuntimeCLI) string {
	if base != "" {
		if rel, err := filepath.Rel(base, path); err == nil && !escapes(rel) {
			return rel
		}
	}
	return format.Path(path, deps.HomeDir)
}

// escapes reports whether a relative path leads outside its base.
func escapes(rel string) bool {
	return rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// renderGroup renders a candidate Project's name, path and repository count,
// noting when it joins an existing project.
func renderGroup(group discover.Group, deps app.RuntimeCLI) string {
	count := deps.Theme.StatusDim.Render(fmt.Sprintf("%d %s", len(group.Repos),
		plural(len(group.Repos), "repository", "repositories")))
	if group.Existing != "" {
		return fmt.Sprintf("%s %s %s",
			deps.Theme.ProjectTitle.Render(group.Existing),
			deps.Theme.RepoPath.Render(format.Path(group.Path, deps.HomeDir)),
			deps.Theme.StatusDim.Render(fmt.Sprintf("+%d new %s", len(group.Repos),
				plural(len(group.Repos), "repository", "repositories"))))
	}
	return fmt.Sprintf("%s %s %s",
		deps.Theme.ProjectTitle.Render(group.Name),
		deps.Theme.RepoPath.Render(format.Path(group.Path, deps.HomeDir)),
		count)
}

// reportTaken warns about each group whose Project name is already taken.
func reportTaken(result discover.Result, deps app.RuntimeCLI) {
	for _, group := range result.Groups {
		if !group.Taken {
			continue
		}
		fmt.Fprintf(deps.Err,
			"%s holds %d repositories, but a project named %q already exists, skipping\n",
			format.Path(group.Path, deps.HomeDir), len(group.Repos), group.Name)
	}
}

// nothingFound explains why a run found nothing to add.
func nothingFound(result discover.Result, root string, minRepos int) string {
	if minRepos <= 0 {
		minRepos = discover.DefaultMin
	}
	switch {
	case result.Total() == 0:
		return fmt.Sprintf("no git repositories found under %s", root)
	case len(result.Groups) > 0:
		// Every name is taken; reportTaken already listed them.
		return "no new projects to add"
	case result.Covered == result.Total():
		return fmt.Sprintf(
			"every repository under %s is already covered by a configured project", root)
	default:
		return fmt.Sprintf(
			"no directory under %s holds %d or more repositories", root, minRepos)
	}
}

// plural picks the form matching n.
func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
