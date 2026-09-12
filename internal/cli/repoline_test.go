package cli

import (
	"errors"
	"fmt"
	"testing"

	"charm.land/lipgloss/v2"
)

// TestRepoLineString pins the one shared result-line format used by the
// fetch/pull/clone/status handlers: "title body" on success and
// "title <styled error>" on failure.
func TestRepoLineString(t *testing.T) {
	title := lipgloss.NewStyle().SetString("myrepo")

	t.Run("success renders title and body", func(t *testing.T) {
		line := RepoLine{Title: title, Body: "[main <- origin/main] ok"}
		want := fmt.Sprintf("%s [main <- origin/main] ok", title)
		if got := line.String(); got != want {
			t.Errorf("String() = %q, want %q", got, want)
		}
	})

	t.Run("error renders styled error instead of body", func(t *testing.T) {
		errStyle := lipgloss.NewStyle()
		line := RepoLine{
			Title:      title,
			Body:       "ignored",
			Err:        errors.New("boom"),
			ErrorStyle: errStyle,
		}
		want := fmt.Sprintf("%s %s", title, errStyle.Render("boom"))
		if got := line.String(); got != want {
			t.Errorf("String() = %q, want %q", got, want)
		}
	})
}
