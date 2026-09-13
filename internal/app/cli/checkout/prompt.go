package checkout

import (
	"fmt"

	"charm.land/huh/v2"
)

// branchPageSize is how many branches the prompt shows at once.
const branchPageSize = 10

// runBranchPrompt runs the branch selection and is the package's one
// interactive seam: everything around it — the state guard, the three outcome
// lines a repository gets once its prompt returns, the traversal and the error
// epilogue — is reachable in a test by replacing this, without a terminal.
//
// choice is the same binding the prompt was built with, passed explicitly so a
// replacement can report a selection by writing through it, exactly as huh
// does. It is unused here.
//
// The field runs as a form rather than through prompt.Run(), which hides the
// help line: it is the only place "/" is advertised as the filter key.
//
// The form is deliberately not given a context, unlike every git call around
// it. Bubbletea handles SIGINT itself and reports it as an abort, which is
// what an interrupted prompt is; handing it the root context would reach the
// same user through huh.ErrTimeout instead.
var runBranchPrompt = func(prompt *huh.Select[string], _ *string) error {
	return huh.NewForm(huh.NewGroup(prompt)).Run()
}

// newBranchPrompt builds the branch selection. choice is huh's value binding:
// the caller seeds it with the branch currently checked out — which the prompt
// opens on, and titles itself with — and it holds the user's pick once the
// prompt has run.
func newBranchPrompt(repoTitle string, branches []string, choice *string) *huh.Select[string] {
	prompt := huh.NewSelect[string]().
		Title(fmt.Sprintf("%s [%s]", repoTitle, *choice)).
		Options(huh.NewOptions(branches...)...).
		Value(choice)

	// Only ask for a height when the list needs paging. huh pads a field out
	// to the height it is given — blank rows a project-wide run would repeat
	// for every repository — whereas an unset height sizes the field to its
	// options exactly, however the title happens to wrap. The extra row is
	// that title, which counts against the height.
	if len(branches) > branchPageSize {
		prompt = prompt.Height(branchPageSize + 1)
	}
	return prompt
}
