package cli

import (
	"fmt"

	"charm.land/lipgloss/v2"
)

// RepoLine is the one result line rendered by the per-repo bulk commands
// (fetch, pull, clone, status): the padded repo title followed by either the
// styled error or the command-specific body.
type RepoLine struct {
	Title      lipgloss.Style
	Body       string
	Err        error
	ErrorStyle lipgloss.Style
}

func (r RepoLine) String() string {
	if r.Err != nil {
		return fmt.Sprintf("%s %s", r.Title, r.ErrorStyle.Render(r.Err.Error()))
	}
	return fmt.Sprintf("%s %s", r.Title, r.Body)
}
