package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/rafi/gits/internal/cli/config"
)

// TestWriteWarningRendersProse proves a downgraded warning reaches Diagnostic
// Output as a plain sentence — not a logrus record. The old path emitted
// `level=warning msg="…"`; this asserts the message stands alone, styled by
// the theme's Warning style and nothing more.
func TestWriteWarningRendersProse(t *testing.T) {
	t.Parallel()

	theme := config.NewThemeDefault()
	var buf bytes.Buffer

	writeWarning(&buf, theme, "no branch selected")

	got := ansi.Strip(buf.String())
	if strings.Contains(got, "level=warning") || strings.Contains(got, "msg=") {
		t.Errorf("warning output = %q, want a plain sentence, not a log record", got)
	}
	if strings.TrimSpace(got) != "no branch selected" {
		t.Errorf("warning output = %q, want %q", got, "no branch selected")
	}
	if !strings.HasSuffix(buf.String(), "\n") {
		t.Errorf("warning output = %q, want a trailing newline", buf.String())
	}
}

// TestNewRuntimeReportsBadDuration proves an unparseable duration setting is
// returned as a warning for the caller to render, rather than logged from the
// domain package. The runtime is still built with the default in place.
//
//nolint:paralleltest // mutates the package-level configFile; must stay serial.
func TestNewRuntimeReportsBadDuration(t *testing.T) {
	orig := configFile
	t.Cleanup(func() { configFile = orig })

	configFile.Settings.CacheTTL = "not-a-duration"
	configFile.Settings.GitTimeout = ""
	configFile.Settings.ProviderTimeout = ""

	_, warnings := newRuntime(t.Context())
	if len(warnings) != 1 {
		t.Fatalf("warnings = %v, want one for the bad cacheTTL", warnings)
	}
	if !strings.Contains(warnings[0].Error(), "cacheTTL") {
		t.Errorf("warning = %v, want it to name the cacheTTL setting", warnings[0])
	}
}
