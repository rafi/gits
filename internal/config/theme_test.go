package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// load writes body as a YAML config and loads it.
func loadYAML(t *testing.T, body string) File {
	t.Helper()
	path := filepath.Join(t.TempDir(), "gits.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	var f File
	if err := NewConfigFromFile(path, &f); err != nil {
		t.Fatalf("load: %v", err)
	}
	return f
}

func TestThemeNameString(t *testing.T) {
	t.Parallel()
	f := loadYAML(t, "settings:\n  theme:\n    name: nord\n")
	if got := f.Settings.Theme.Name; got.Dark != "nord" || got.Light != "nord" {
		t.Errorf("Name = %+v, want nord for both", got)
	}
	if len(f.Warnings) != 0 {
		t.Errorf("Warnings = %v, want none", f.Warnings)
	}
}

func TestThemeNamePair(t *testing.T) {
	t.Parallel()
	f := loadYAML(t, "settings:\n  theme:\n    name: {dark: catppuccin-mocha, light: catppuccin-latte}\n")
	if got := f.Settings.Theme.Name; got.Dark != "catppuccin-mocha" || got.Light != "catppuccin-latte" {
		t.Errorf("Name = %+v", got)
	}
}

func TestThemeStyles(t *testing.T) {
	t.Parallel()
	f := loadYAML(t, "settings:\n  theme:\n    styles:\n      statusStaged: {fg: green, bold: true}\n      statusBehind: {faint: false}\n      statusGone: {fg: 5}\n")
	st := f.Settings.Theme.Styles
	if s := st["statusStaged"]; s.Fg == nil || *s.Fg != "green" || s.Bold == nil || !*s.Bold {
		t.Errorf("statusStaged = %+v", s)
	}
	if s := st["statusBehind"]; s.Faint == nil || *s.Faint {
		t.Errorf("statusBehind faint = %v, want explicit false", s.Faint)
	}
	if s := st["statusGone"]; s.Fg == nil || *s.Fg != "5" {
		t.Errorf("statusGone fg = %v, want \"5\" from an unquoted int", s.Fg)
	}
}

func TestThemeLegacyFlatForm(t *testing.T) {
	t.Parallel()
	f := loadYAML(t, "settings:\n  theme:\n    repoTitle: {color: \"4\", bold: true}\n    diff: {color: \"140\", align: right, width: 5}\n")
	st := f.Settings.Theme.Styles
	if s := st["repoTitle"]; s.Fg == nil || *s.Fg != "4" || s.Bold == nil || !*s.Bold {
		t.Errorf("repoTitle = %+v, want fg 4 bold", s)
	}
	if s := st["diff"]; s.Fg == nil || *s.Fg != "140" {
		t.Errorf("diff = %+v, want fg 140 with align/width dropped", s)
	}
	n := 0
	for _, w := range f.Warnings {
		if strings.Contains(w, "deprecated") && strings.Contains(w, "styles:") {
			n++
		}
		if strings.Contains(w, "unknown config key") {
			t.Errorf("legacy keys reported as unknown: %q", w)
		}
	}
	if n != 1 {
		t.Errorf("Warnings = %v, want exactly one deprecation notice", f.Warnings)
	}
}

func TestThemeEmpty(t *testing.T) {
	t.Parallel()
	f := loadYAML(t, "settings: {}\n")
	if f.Settings.Theme.Name.Dark != "" || len(f.Settings.Theme.Styles) != 0 {
		t.Errorf("Theme = %+v, want zero (resolves to ansi later)", f.Settings.Theme)
	}
}
