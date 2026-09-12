package config

import "testing"

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
