// Package list implements `gits list`, which renders a Project's Repositories
// as a table, a tree, a name list, or a JSON document.
package list

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/loader"
	"github.com/rafi/gits/internal/types"
)

// lister renders the loaded projects in one output style.
type lister func(domain.ProjectListKeyed, types.RuntimeCLI) error

// style is what one `-o` value selects: the renderer to run, and whether that
// renderer displays each repository's Repo Src.
type style struct {
	// lister is chosen from the arguments, since `name` answers a different
	// question when a project is named than when none is.
	lister func(args []string) lister
	// showsSrc marks a style that prints each repository's Repo Src.
	// Resolving one costs a git subprocess per Repository, so only the styles
	// that display it pay for it.
	showsSrc bool
}

// styles is the one place `list -o` is defined: ExecList dispatches through
// it, a rejected value is reported against its keys, and the flag's help text
// and shell completion are built from them — so a style cannot be added in
// one place and missed in another.
var styles = map[string]style{
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
func ExecList(format string, args []string, deps types.RuntimeCLI) error {
	// Validated before anything is loaded, so a typo'd format never costs a
	// provider round-trip or an interactive prompt.
	st, ok := styles[format]
	if !ok {
		return fmt.Errorf("unknown output format %q, want one of %s",
			format, strings.Join(Formats(), ", "))
	}

	projects, err := loader.GetProjects(args, deps.Runtime)
	if err != nil {
		return err
	}
	if len(projects) == 0 {
		return types.NewWarning(
			`No projects found.
Either your %q is empty, or you misspelled the project name.`,
			deps.ConfigPath,
		)
	}
	// Resolve each Repo Src now — lazily, only for the styles that show one.
	// `name` and `tree` never print a source and so never pay the
	// per-repository git subprocess resolution costs.
	if st.showsSrc {
		loader.ResolveProjectsSrc(deps.Ctx, deps.Git, projects)
	}
	return st.lister(args)(projects, deps)
}

// fixed adapts a renderer that does not depend on the arguments.
func fixed(l lister) func([]string) lister {
	return func([]string) lister { return l }
}

// nameLister picks between the two name forms: with no project named, `name`
// lists the project names; with one named, it lists that project's
// repositories.
func nameLister(args []string) lister {
	if len(args) > 0 {
		return listNameRepos
	}
	return listNameProjects
}
