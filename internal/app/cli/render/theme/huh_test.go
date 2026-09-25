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
