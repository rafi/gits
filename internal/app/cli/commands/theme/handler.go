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
		t, _ := rtheme.Lookup(n)
		s := rtheme.Scheme{Theme: t, Profile: deps.Scheme.Profile}
		mark := " "
		if n == deps.Scheme.Theme.Name {
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
	}
	deps.Scheme = s
	deps.Theme = rtheme.Build(s.Theme, s.Profile, nil)

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
