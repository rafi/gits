package theme

import (
	"fmt"
	"image/color"
	"io"
	"sort"
	"strings"

	"github.com/charmbracelet/colorprofile"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/app/cli/render/style"
)

// Scheme is the resolved theme and the color profile its styles were built
// for: what adapters (huh, glamour, `gits theme`) derive their colors from.
type Scheme struct {
	Theme   Theme
	Profile colorprofile.Profile
}

// Color resolves a palette slot for the scheme's profile.
func (s Scheme) Color(slot Slot) color.Color { return s.Theme.Palette.Get(slot).resolve(s.Profile) }

// ResolveInput is everything theme resolution reads. IsDark asks the terminal
// for its background; it is called only when the configured dark and light
// themes differ.
type ResolveInput struct {
	Flag, Env      string
	Settings       domain.ThemeSettings
	Color          string // the --color value: auto, always or never
	Stdout, Stderr io.Writer
	Environ        []string
	IsDark         func() bool
}

// Resolve picks the theme (flag > env > config > ansi), the color profile and
// builds the styles. Problems in config or env are warnings with a fallback;
// an unknown --theme is the one hard error.
func Resolve(in ResolveInput) (Scheme, style.Theme, []error, error) {
	var warns []error
	name, fromFlag := pickName(in)
	t, err := Lookup(name)
	switch {
	case err != nil && fromFlag:
		return Scheme{}, style.Theme{}, nil, err
	case err != nil:
		warns = append(warns, fmt.Errorf("%w; using ansi", err))
		t = Ansi
	}

	overrides, bad := ParseStyles(in.Settings.Styles)
	warns = append(warns, bad...)

	s := Scheme{Theme: t, Profile: profile(in)}
	return s, Build(t, s.Profile, overrides), warns, nil
}

// pickName applies the selection precedence and reports whether the name came
// from the flag, whose errors are fatal.
func pickName(in ResolveInput) (string, bool) {
	switch {
	case in.Flag != "":
		return in.Flag, true
	case in.Env != "":
		return in.Env, false
	}
	dark, light := in.Settings.Name.Dark, in.Settings.Name.Light
	switch {
	case dark == "" && light == "":
		return Ansi.Name, false
	case dark == "":
		return light, false
	case light == "" || dark == light:
		return dark, false
	case in.IsDark():
		return dark, false
	default:
		return light, false
	}
}

// profile is the forced profile for --color always/never, else the richer of
// the two output streams' detected profiles: styles are built at that level
// and each stream's writer downsamples to its own.
func profile(in ResolveInput) colorprofile.Profile {
	switch in.Color {
	case "never":
		return colorprofile.NoTTY
	case "always":
		return colorprofile.TrueColor
	}
	return max(colorprofile.Detect(in.Stdout, in.Environ), colorprofile.Detect(in.Stderr, in.Environ))
}

// ParseStyles validates settings.theme.styles into role overrides, skipping
// (with a warning) unknown roles and invalid colors so one typo does not cost
// the rest of the config.
func ParseStyles(cfg map[string]domain.StyleSpec) (map[Role]Spec, []error) {
	valid := map[Role]bool{}
	for _, r := range Roles() {
		valid[r] = true
	}
	keys := make([]string, 0, len(cfg))
	for k := range cfg {
		keys = append(keys, k)
	}
	sort.Strings(keys) // stable warning order

	out := map[Role]Spec{}
	var warns []error
	for _, key := range keys {
		if !valid[Role(key)] {
			warns = append(warns, fmt.Errorf("settings.theme.styles: unknown role %q, ignored; valid roles: %s",
				key, strings.Join(roleNames(), ", ")))
			continue
		}
		spec, err := SpecFromConfig(cfg[key])
		if err != nil {
			warns = append(warns, fmt.Errorf("settings.theme.styles.%s: %w, ignored", key, err))
			continue
		}
		out[Role(key)] = spec
	}
	return out, warns
}

// SpecFromConfig converts one config override, validating its colors.
func SpecFromConfig(c domain.StyleSpec) (Spec, error) {
	s := Spec{Bold: c.Bold, Faint: c.Faint, Italic: c.Italic, Underline: c.Underline}
	var err error
	if c.Fg != nil {
		if s.Fg, err = ParseColorRef(*c.Fg); err != nil {
			return Spec{}, err
		}
	}
	if c.Bg != nil {
		if s.Bg, err = ParseColorRef(*c.Bg); err != nil {
			return Spec{}, err
		}
	}
	return s, nil
}

func roleNames() []string {
	names := make([]string, 0, len(Roles()))
	for _, r := range Roles() {
		names = append(names, string(r))
	}
	return names
}
