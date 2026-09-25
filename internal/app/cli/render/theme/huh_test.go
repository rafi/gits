package theme

import (
	"strings"
	"testing"

	"github.com/charmbracelet/colorprofile"
)

func TestHuhUsesAccent(t *testing.T) {
	t.Parallel()
	s := Scheme{Theme: Dracula, Profile: colorprofile.TrueColor}
	st := Huh(s).Theme(true)
	// dracula accent #bd93f9 = 189;147;249
	for name, got := range map[string]string{
		"title":    st.Focused.Title.Render("x"),
		"selector": st.Focused.SelectSelector.Render(),
	} {
		if !strings.Contains(got, "38;2;189;147;249") {
			t.Errorf("focused %s = %q, want the accent", name, got)
		}
	}
}

// TestHuhAnsiOptionsLeaveForegroundUnset proves the ansi theme's options
// render with no SGR foreground, so the terminal's own text color shows
// through instead of the ansi palette's Fg (ANSI 15 — near-invisible on a
// light terminal). A named theme still colors its options.
func TestHuhAnsiOptionsLeaveForegroundUnset(t *testing.T) {
	t.Parallel()
	st := Huh(Scheme{Theme: Ansi, Profile: colorprofile.TrueColor}).Theme(true)
	for name, got := range map[string]string{
		"option":           st.Focused.Option.Render("x"),
		"unselectedOption": st.Focused.UnselectedOption.Render("x"),
	} {
		if got != "x" {
			t.Errorf("ansi focused %s = %q, want plain %q (no SGR)", name, got, "x")
		}
	}

	named := Huh(Scheme{Theme: Dracula, Profile: colorprofile.TrueColor}).Theme(true)
	if got := named.Focused.Option.Render("x"); got == "x" {
		t.Error("dracula focused option has no SGR, want it colored")
	}
}
