package theme

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/colorprofile"

	"github.com/rafi/gits/internal/app/cli/render/style"
)

// renderAll renders a sample through every role, one "role: <sgr>x" line
// each, so a golden diff names the role that drifted.
func renderAll(th style.Theme) string {
	var b strings.Builder
	for _, r := range Roles() {
		fmt.Fprintf(&b, "%s: %q\n", r, roleFields[r](&th).Render("x"))
	}
	return b.String()
}

// TestAnsiMatchesLegacyDefault pins the promise that not choosing a theme
// changes nothing: the ansi theme renders every role exactly as the
// pre-theme defaults did. The golden was captured from the pre-theme
// defaults and must never be regenerated from the ansi theme itself.
func TestAnsiMatchesLegacyDefault(t *testing.T) {
	t.Parallel()
	path := filepath.Join("testdata", "ansi.golden")
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden: %v (frozen from the pre-theme defaults; never regenerate it)", err)
	}
	if got := renderAll(Build(Ansi, colorprofile.TrueColor, nil)); got != string(want) {
		t.Errorf("ansi theme drifted from the legacy default:\n got:\n%s\nwant:\n%s", got, want)
	}
}
