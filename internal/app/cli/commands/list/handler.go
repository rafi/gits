// Package list implements `gits list`, which renders a Project's Repositories
// as a table, a tree, a name list, or a JSON document.
package list

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/app"
	"github.com/rafi/gits/internal/runtime/projects"
)

// lister renders the loaded projects in one output style.
type lister func(domain.ProjectListKeyed, app.RuntimeCLI) error

// outputStyle is what one `-o` value selects: the renderer to run, and
// whether that renderer displays each repository's Repo Src.
type outputStyle struct {
	// lister is chosen from the arguments, since `name` answers a different
	// question when a project is named than when none is.
	lister func(args []string, tags domain.TagSet) lister
	// showsSrc marks a style that prints each repository's Repo Src.
	// Resolving one costs a git subprocess per Repository, so only the styles
	// that display it pay for it.
	showsSrc bool
}

// styles is the one place `list -o` is defined: ExecList dispatches through
// it, a rejected value is reported against its keys, and the flag's help text
// and shell completion are built from them — so a style cannot be added in
// one place and missed in another.
var styles = map[string]outputStyle{
	"json":  {lister: fixed(listJSON), showsSrc: true},
	"name":  {lister: nameLister},
	"table": {lister: fixed(listTable), showsSrc: true},
	"tree":  {lister: fixed(listTree)},
	"wide":  {lister: fixed(listWide), showsSrc: true},
}

// Formats are the output styles `list` accepts, sorted, for the flag's help
// text and its shell completion.
func Formats() []string {
	return slices.Sorted(maps.Keys(styles))
}

// ExecList displays a list of projects and repositories.
//
// Args: (optional)
//   - project names
func ExecList(format string, tags domain.TagSet, args []string, deps app.RuntimeCLI) error {
	// Validated before anything is loaded, so a typo'd format never costs a
	// provider round-trip or an interactive prompt.
	st, ok := styles[format]
	if !ok {
		return fmt.Errorf("unknown output format %q, want one of %s",
			format, strings.Join(Formats(), ", "))
	}

	loaded, err := projects.Load(args, deps.Runtime, projects.WithTags(tags))
	if err != nil {
		return err
	}
	if len(loaded) == 0 {
		return domain.NewWarning(
			`No projects found.
Either your %q is empty, or you misspelled the project name.`,
			deps.ConfigPath,
		)
	}
	// Warn when no repository carries the tag.
	if !tags.Empty() && countRepos(loaded) == 0 {
		return domain.NewWarning("no repository carries tag %s", tags)
	}
	// Hide projects the tag filter emptied.
	if !tags.Empty() {
		dropEmpty(loaded)
	}
	// Resolve each Repo Src now — lazily, only for the styles that show one.
	// `name` and `tree` never print a source and so never pay the
	// per-repository git subprocess resolution costs.
	if st.showsSrc {
		projects.FillSourcesKeyed(deps.Ctx, deps.Git, loaded)
	}
	return st.lister(args, tags)(loaded, deps)
}

// countRepos returns the number of repositories in list, Sub-projects
// included.
func countRepos(list domain.ProjectListKeyed) int {
	total := 0
	for _, proj := range list {
		total += proj.CountRepos()
	}
	return total
}

// dropEmpty removes projects holding no repositories.
func dropEmpty(list domain.ProjectListKeyed) {
	for name, proj := range list {
		if proj.CountRepos() == 0 {
			delete(list, name)
		}
	}
}

// fixed adapts a renderer that does not depend on the arguments.
func fixed(l lister) func([]string, domain.TagSet) lister {
	return func([]string, domain.TagSet) lister { return l }
}

// nameLister picks between the two name forms: with no project named, `name`
// lists the project names; with one named, it lists that project's
// repositories.
//
// With a `--tag`, it lists repositories.
func nameLister(args []string, tags domain.TagSet) lister {
	if len(args) > 0 || !tags.Empty() {
		return listNameRepos
	}
	return listNameProjects
}
