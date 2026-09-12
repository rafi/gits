package config

import (
	"strings"
	"testing"
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
