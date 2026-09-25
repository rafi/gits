package theme

import (
	"bytes"
	"strings"
	"testing"

	"github.com/charmbracelet/colorprofile"

	"github.com/rafi/gits/domain"
)

func in(flag, env string, dark, light string) ResolveInput {
	return ResolveInput{
		Flag: flag, Env: env,
		Settings: domain.ThemeSettings{Name: domain.ThemeName{Dark: dark, Light: light}},
		Color:    "always",
		Stdout:   &bytes.Buffer{}, Stderr: &bytes.Buffer{},
		IsDark: func() bool { panic("background probed") },
	}
}

func TestResolvePrecedence(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   ResolveInput
		want string
	}{
		{"default", in("", "", "", ""), "ansi"},
		{"config", in("", "", "nord", "nord"), "nord"},
		{"env beats config", in("", "dracula", "nord", "nord"), "dracula"},
		{"flag beats env", in("one-dark", "dracula", "nord", "nord"), "one-dark"},
		{"one half set", in("", "", "nord", ""), "nord"},
	}
	for _, c := range cases {
		s, _, warns, err := Resolve(c.in)
		if err != nil || len(warns) != 0 || s.Theme.Name != c.want {
			t.Errorf("%s: theme %q, warns %v, err %v; want %q", c.name, s.Theme.Name, warns, err, c.want)
		}
	}
}

func TestResolvePairProbesBackground(t *testing.T) {
	t.Parallel()
	for _, dark := range []bool{true, false} {
		r := in("", "", "catppuccin-mocha", "catppuccin-latte")
		r.IsDark = func() bool { return dark }
		s, _, _, _ := Resolve(r)
		want := map[bool]string{true: "catppuccin-mocha", false: "catppuccin-latte"}[dark]
		if s.Theme.Name != want {
			t.Errorf("dark=%v: theme %q, want %q", dark, s.Theme.Name, want)
		}
	}
}

func TestResolveBadInputs(t *testing.T) {
	t.Parallel()
	if _, _, _, err := Resolve(in("nope", "", "", "")); err == nil {
		t.Error("bad --theme: err = nil, want hard error")
	}
	s, _, warns, err := Resolve(in("", "nope", "", ""))
	if err != nil || len(warns) != 1 || s.Theme.Name != "ansi" {
		t.Errorf("bad GITS_THEME: theme %q warns %v err %v; want ansi + 1 warning", s.Theme.Name, warns, err)
	}
	s, _, warns, _ = Resolve(in("", "", "nope", "nope"))
	if len(warns) != 1 || s.Theme.Name != "ansi" {
		t.Errorf("bad config theme: theme %q warns %v", s.Theme.Name, warns)
	}
}

func TestResolveProfileFromColorFlag(t *testing.T) {
	t.Parallel()
	r := in("", "", "", "")
	r.Color = "never"
	if s, _, _, _ := Resolve(r); s.Profile != colorprofile.NoTTY {
		t.Errorf("--color never profile = %v, want NoTTY", s.Profile)
	}
	r.Color = "always"
	if s, _, _, _ := Resolve(r); s.Profile != colorprofile.TrueColor {
		t.Errorf("--color always profile = %v, want TrueColor", s.Profile)
	}
}

func TestParseStyles(t *testing.T) {
	t.Parallel()
	green, bad := "green", "purple"
	no := false
	got, warns := ParseStyles(map[string]domain.StyleSpec{
		"statusStaged": {Fg: &green},
		"statusBehind": {Faint: &no},
		"statusGone":   {Fg: &bad},
		"stautsDim":    {Fg: &green},
	})
	if _, ok := got[RoleStatusStaged]; !ok {
		t.Error("statusStaged override dropped")
	}
	if s := got[RoleStatusBehind]; s.Faint == nil || *s.Faint {
		t.Error("statusBehind faint:false not kept")
	}
	if _, ok := got[RoleStatusGone]; ok {
		t.Error("bad color override kept, want skipped")
	}
	if len(warns) != 2 {
		t.Fatalf("warns = %v, want bad color + unknown role", warns)
	}
	joined := warns[0].Error() + warns[1].Error()
	if !strings.Contains(joined, "stautsDim") || !strings.Contains(joined, "statusDim") {
		t.Errorf("unknown-role warning %q should name the key and list valid roles", joined)
	}
}
