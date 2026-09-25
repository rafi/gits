package theme

import (
	"strings"
	"testing"

	"github.com/rafi/gits/internal/app/cli/clitest"
	rtheme "github.com/rafi/gits/internal/app/cli/render/theme"
)

func TestListNonTTYPrintsBareNames(t *testing.T) {
	t.Parallel()
	deps := clitest.New(t, nil)
	if err := ExecList(deps.RuntimeCLI); err != nil {
		t.Fatal(err)
	}
	got := strings.Split(strings.TrimSpace(deps.Result()), "\n")
	if strings.Join(got, ",") != strings.Join(rtheme.Names(), ",") {
		t.Errorf("list = %v, want exactly the sorted names", got)
	}
}

// TestShowRendersEveryGlyph does not run in parallel: it forces the process
// environment so the destination buffer resolves to TrueColor, and a
// concurrent test could observe or race that environment change. Renderers
// write through lipgloss.Fprint*, which wraps the writer in a colorprofile
// detector that downsamples SGR to whatever profile it detects for that
// writer — a captured in-memory buffer is not a terminal, so without
// CLICOLOR_FORCE and COLORTERM it would strip the truecolor escape this test
// asserts on.
func TestShowRendersEveryGlyph(t *testing.T) {
	t.Setenv("CLICOLOR_FORCE", "1")
	t.Setenv("COLORTERM", "truecolor")
	deps := clitest.New(t, nil)
	if err := ExecShow([]string{"dracula"}, deps.RuntimeCLI); err != nil {
		t.Fatal(err)
	}
	ic := deps.Settings.Icons
	for _, g := range []string{ic.Staged, ic.Unstaged, ic.Untracked, ic.Ahead,
		ic.Behind, ic.Diverged, ic.Gone, ic.DiffError, ic.NA} {
		if !strings.Contains(deps.Result(), g) {
			t.Errorf("show output lacks glyph %q", g)
		}
	}
	// Raw (unstripped) output carries dracula's green #50fa7b at TrueColor.
	if raw := deps.RawResult(); !strings.Contains(raw, "38;2;80;250;123") {
		t.Error("show dracula output has no dracula truecolor SGR")
	}
}

func TestShowUnknownErrors(t *testing.T) {
	t.Parallel()
	deps := clitest.New(t, nil)
	err := ExecShow([]string{"nope"}, deps.RuntimeCLI)
	if err == nil || !strings.Contains(err.Error(), "dracula") {
		t.Errorf("err = %v, want unknown-theme error listing names", err)
	}
}
