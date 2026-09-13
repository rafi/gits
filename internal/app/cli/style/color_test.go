package style

import (
	"os"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
)

func TestColorOptionString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		opt  colorOption
		want string
	}{
		{ColorOptionAuto, "auto"},
		{ColorOptionAlways, "always"},
		{ColorOptionNever, "never"},
	}
	for _, tt := range tests {
		if got := tt.opt.String(); got != tt.want {
			t.Errorf("colorOption(%d).String() = %q, want %q", tt.opt, got, tt.want)
		}
	}
	if ColorOptionDefault != "auto" {
		t.Errorf("ColorOptionDefault = %q, want %q", ColorOptionDefault, "auto")
	}
}

// TestColorValueSet proves the --color flag rejects anything but the three
// accepted values, so a typo fails loudly instead of silently meaning auto.
func TestColorValueSet(t *testing.T) {
	t.Parallel()

	t.Run("accepts the three valid values", func(t *testing.T) {
		t.Parallel()

		for _, v := range []string{"auto", "always", "never"} {
			var dst string
			cv := NewColorValue(&dst)
			if err := cv.Set(v); err != nil {
				t.Errorf("Set(%q) = %v, want nil", v, err)
			}
			if dst != v {
				t.Errorf("after Set(%q), dst = %q, want %q", v, dst, v)
			}
			if cv.String() != v {
				t.Errorf("String() = %q, want %q", cv.String(), v)
			}
		}
	})

	t.Run("rejects an unknown value and lists the valid ones", func(t *testing.T) {
		t.Parallel()

		var dst string
		cv := NewColorValue(&dst)
		err := cv.Set("sometimes")
		if err == nil {
			t.Fatal("Set(\"sometimes\") = nil, want an error")
		}
		for _, want := range []string{"auto", "always", "never"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error = %v, want it to list %q", err, want)
			}
		}
		// A rejected value must not overwrite the seeded default.
		if dst != ColorOptionDefault {
			t.Errorf("dst = %q after rejected Set, want the default %q", dst, ColorOptionDefault)
		}
	})

	t.Run("defaults are seeded and Type is stable", func(t *testing.T) {
		t.Parallel()

		var dst string
		cv := NewColorValue(&dst)
		if dst != ColorOptionDefault {
			t.Errorf("NewColorValue seeded dst = %q, want %q", dst, ColorOptionDefault)
		}
		if cv.Type() != "color" {
			t.Errorf("Type() = %q, want %q", cv.Type(), "color")
		}
	})
}

// TestApplyColor pins the two forcing values: "never" and "always" must reach
// both the global lipgloss writer and the environment, so a child process
// (git, fzf) and a per-writer output agree with the flag.
//
//nolint:paralleltest // restoreEnv rewrites process environment; must stay serial.
func TestApplyColor(t *testing.T) {
	origProfile := lipgloss.Writer.Profile
	origNoColor, hadNoColor := os.LookupEnv("NO_COLOR")
	origForce, hadForce := os.LookupEnv("CLICOLOR_FORCE")
	t.Cleanup(func() {
		lipgloss.Writer.Profile = origProfile
		restoreEnv(t, "NO_COLOR", origNoColor, hadNoColor)
		restoreEnv(t, "CLICOLOR_FORCE", origForce, hadForce)
	})

	t.Run("never forces NoTTY", func(t *testing.T) {
		os.Unsetenv("NO_COLOR")
		ApplyColor(ColorOptionNever.String())
		if lipgloss.Writer.Profile != colorprofile.NoTTY {
			t.Errorf("profile = %v, want NoTTY", lipgloss.Writer.Profile)
		}
		if os.Getenv("NO_COLOR") != "1" {
			t.Errorf("NO_COLOR = %q, want 1", os.Getenv("NO_COLOR"))
		}
	})

	t.Run("always forces TrueColor", func(t *testing.T) {
		os.Unsetenv("CLICOLOR_FORCE")
		ApplyColor(ColorOptionAlways.String())
		if lipgloss.Writer.Profile != colorprofile.TrueColor {
			t.Errorf("profile = %v, want TrueColor", lipgloss.Writer.Profile)
		}
		if os.Getenv("CLICOLOR_FORCE") != "1" {
			t.Errorf("CLICOLOR_FORCE = %q, want 1", os.Getenv("CLICOLOR_FORCE"))
		}
	})

	t.Run("auto leaves detection alone", func(t *testing.T) {
		lipgloss.Writer.Profile = colorprofile.Ascii
		ApplyColor(ColorOptionAuto.String())
		if lipgloss.Writer.Profile != colorprofile.Ascii {
			t.Errorf("profile = %v, want unchanged", lipgloss.Writer.Profile)
		}
	})
}

func restoreEnv(t *testing.T, key, val string, had bool) {
	t.Helper()
	if had {
		//nolint:usetesting // t.Setenv registers its own cleanup and cannot run from inside one.
		os.Setenv(key, val)
	} else {
		os.Unsetenv(key)
	}
}
