package theme

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
)

var testTheme = Theme{
	Name: "test",
	Dark: true,
	Palette: Palette{
		Fg: C("#eeeeee", 255, 7), Muted: C("#888888", 102, 8), Subtle: C("#444444", 238, 8),
		Red: C("#ff0000", 196, 1), Green: C("#00ff00", 46, 2), Yellow: C("#ffff00", 226, 3),
		Blue: C("#0000ff", 21, 4), Magenta: C("#ff00ff", 201, 5), Cyan: C("#00ffff", 51, 6),
		Orange: C("#ff8800", 208, 3), Accent: C("#ff00ff", 201, 5),
	},
}

func TestBuildPicksTrioMemberPerProfile(t *testing.T) {
	t.Parallel()
	cases := []struct {
		p    colorprofile.Profile
		want string // SGR fragment for Green
	}{
		{colorprofile.TrueColor, "38;2;0;255;0"},
		{colorprofile.ANSI256, "38;5;46"},
		{colorprofile.ANSI, "32"},
	}
	for _, c := range cases {
		got := Build(testTheme, c.p, nil).StatusStaged.Render("x")
		if !strings.Contains(got, c.want) {
			t.Errorf("profile %v: StatusStaged = %q, want SGR %q", c.p, got, c.want)
		}
	}
}

func TestBuildNoTTYHasNoColor(t *testing.T) {
	t.Parallel()
	got := Build(testTheme, colorprofile.NoTTY, nil).StatusStaged.GetForeground()
	if _, ok := got.(lipgloss.NoColor); !ok {
		t.Errorf("NoTTY StatusStaged foreground = %#v, want NoColor", got)
	}
}

func TestBuildLayersTweaksThenOverrides(t *testing.T) {
	t.Parallel()
	yes, no := true, false
	th := testTheme
	th.Tweaks = map[Role]Spec{RoleStatusStaged: {Bold: &yes}}
	over := map[Role]Spec{RoleStatusStaged: {Fg: Ref(SlotRed)}, RoleStatusBehind: {Faint: &no}}
	built := Build(th, colorprofile.TrueColor, over)
	if s := built.StatusStaged.Render("x"); !strings.Contains(s, "38;2;255;0;0") || !built.StatusStaged.GetBold() {
		t.Errorf("StatusStaged = %q, want red (override) and bold (tweak)", s)
	}
	if built.StatusBehind.GetFaint() {
		t.Error("faint: false override did not clear the mapping's faint")
	}
}

func TestBuildRawAndNoneRefs(t *testing.T) {
	t.Parallel()
	over := map[Role]Spec{RoleStatusGone: {Fg: Raw("#123456")}, RoleStatusDim: {Fg: None()}}
	built := Build(testTheme, colorprofile.TrueColor, over)
	if s := built.StatusGone.Render("x"); !strings.Contains(s, "38;2;18;52;86") {
		t.Errorf("StatusGone = %q, want the raw hex", s)
	}
	if _, ok := built.StatusDim.GetForeground().(lipgloss.NoColor); !ok {
		t.Error("fg: none did not clear StatusDim's foreground")
	}
	// A raw color at ANSI degrades through colorprofile rather than vanishing.
	low := Build(testTheme, colorprofile.ANSI, over).StatusGone.GetForeground()
	if _, ok := low.(lipgloss.NoColor); ok {
		t.Error("raw hex at ANSI resolved to no color, want a degraded color")
	}
}
