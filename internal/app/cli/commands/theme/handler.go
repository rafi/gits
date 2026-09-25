// Package theme is `gits theme`: listing the compiled-in themes and previewing
// one through the real renderers.
package theme

import (
	"fmt"
	"os"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/term"

	"github.com/rafi/gits/internal/app"
	"github.com/rafi/gits/internal/app/cli/commands/list"
	"github.com/rafi/gits/internal/app/cli/commands/status"
	rtheme "github.com/rafi/gits/internal/app/cli/render/theme"
)

// swatchSlots are the hues `list` shows per theme.
var swatchSlots = []rtheme.Slot{
	rtheme.SlotRed, rtheme.SlotGreen, rtheme.SlotYellow, rtheme.SlotBlue,
	rtheme.SlotMagenta, rtheme.SlotCyan, rtheme.SlotOrange, rtheme.SlotAccent,
}

// ExecList prints one row per theme with swatches in its own colors, or bare
// names when Result Output is not a terminal, so the list pipes into fzf.
func ExecList(deps app.RuntimeCLI) error {
	if f, ok := deps.Out.(*os.File); !ok || !term.IsTerminal(f.Fd()) {
		for _, n := range rtheme.Names() {
			fmt.Fprintln(deps.Out, n)
		}
		return nil
	}
	width := 0
	for _, n := range rtheme.Names() {
		width = max(width, len(n))
	}
	for _, n := range rtheme.Names() {
		// n came from Names(), the same slice Lookup searches, so this cannot
		// fail.
		t, _ := rtheme.Lookup(n)
		s := rtheme.Scheme{Theme: t, Profile: deps.Scheme.Profile}
		mark := " "
		if isActiveTheme(n, deps) {
			mark = "*"
		}
		bg := "light"
		switch {
		case n == rtheme.Ansi.Name:
			bg = "·"
		case t.Dark:
			bg = "dark"
		}
		var sw strings.Builder
		for _, slot := range swatchSlots {
			sw.WriteString(lipgloss.NewStyle().Foreground(s.Color(slot)).Render("███") + " ")
		}
		lipgloss.Fprintf(deps.Out, "%s %-*s  %-5s  %s\n", mark, width, n, bg, strings.TrimRight(sw.String(), " "))
	}
	return nil
}

// isActiveTheme reports whether name gets the `*` marker in `theme list`. The
// spec marks the active theme(s): the one this run actually resolved to, plus
// both halves of a configured {dark, light} pair, since the other half is
// exactly as "active" the moment the terminal's background differs from what
// this run saw. Whether a flag or GITS_THEME override is in effect is not
// tracked this far from Resolve, so this deliberately stays simple rather
// than trying to detect that case.
func isActiveTheme(name string, deps app.RuntimeCLI) bool {
	return name == deps.Scheme.Theme.Name ||
		name == deps.Settings.Theme.Name.Dark ||
		name == deps.Settings.Theme.Name.Light
}

// ExecShow renders sample output — a status table, a project tree, a warning,
// an error and the palette — in the named theme, or the active one.
func ExecShow(args []string, deps app.RuntimeCLI) error {
	s := deps.Scheme
	if len(args) > 0 {
		t, err := rtheme.Lookup(args[0])
		if err != nil {
			return err
		}
		s.Theme = t
		// The user's settings.theme.styles overrides apply to whichever theme
		// is shown, not only the active one, so a named theme's preview looks
		// like it would if the user actually switched to it. Any warnings
		// (unknown role, bad color) were already reported once at startup, so
		// they are dropped here rather than shown a second time.
		overrides, _ := rtheme.ParseStyles(deps.Settings.Theme.Styles)
		deps.Scheme = s
		deps.Theme = rtheme.Build(s.Theme, s.Profile, overrides)
	}

	lipgloss.Fprintln(deps.Out, deps.Theme.StatusHeader.Render("Theme: "+s.Theme.Name))
	fmt.Fprintln(deps.Out)
	status.RenderSample(deps)
	fmt.Fprintln(deps.Out)
	list.RenderSample(deps)
	fmt.Fprintln(deps.Out)
	lipgloss.Fprintln(deps.Out, deps.Theme.Warning.Render("warning: api has no upstream"))
	lipgloss.Fprintln(deps.Out, deps.Theme.Error.Render("error: web: not a readable git repository"))
	fmt.Fprintln(deps.Out)
	for _, slot := range rtheme.Slots() {
		lipgloss.Fprintf(deps.Out, "%s %s\n",
			lipgloss.NewStyle().Foreground(s.Color(slot)).Render("███"), slot)
	}
	return nil
}
