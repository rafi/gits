package theme

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"

	"github.com/rafi/gits/internal/app/cli/render/style"
)

var update = flag.Bool("update", false, "rewrite testdata/ansi.golden")

// renderAll renders a sample through every role, one "role: <sgr>x" line
// each, so a golden diff names the role that drifted.
func renderAll(th style.Theme) string {
	var b strings.Builder
	for _, r := range Roles() {
		fmt.Fprintf(&b, "%s: %q\n", r, roleFields[r](&th).Render("x"))
	}
	return b.String()
}

// legacyProgress is today's hard-coded progress palette (progress/live.go
// newStyles), which the ansi theme must keep.
func legacyProgress(th *style.Theme) {
	th.ProgressName = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	th.ProgressPercent = lipgloss.NewStyle().Foreground(lipgloss.Color("14"))
	th.ProgressFill = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	th.ProgressEmpty = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	th.ProgressStats = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	// The spinner intentionally moves from huh's #F780E2 to ANSI magenta.
	th.ProgressSpinner = lipgloss.NewStyle().Foreground(lipgloss.Color("5"))
}

// TestAnsiMatchesLegacyDefault pins the promise that not choosing a theme
// changes nothing: the ansi theme renders every role exactly as the
// pre-theme defaults did. Regenerate only from the legacy theme:
// go test ./internal/app/cli/render/theme/ -run TestAnsiMatchesLegacyDefault -update
func TestAnsiMatchesLegacyDefault(t *testing.T) {
	path := filepath.Join("testdata", "ansi.golden")
	if *update {
		legacy := style.NewThemeDefault()
		legacyProgress(&legacy)
		if err := os.WriteFile(path, []byte(renderAll(legacy)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden: %v (run with -update once, before Task 5)", err)
	}
	if got := renderAll(Build(Ansi, colorprofile.TrueColor, nil)); got != string(want) {
		t.Errorf("ansi theme drifted from the legacy default:\n got:\n%s\nwant:\n%s", got, want)
	}
}
