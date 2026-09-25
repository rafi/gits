package theme

import (
	"fmt"
	"image/color"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
)

// ColorRef is a color as a style names it: a palette slot (which follows the
// theme), a raw hex or ANSI index (which colorprofile degrades), or none.
type ColorRef struct {
	slot Slot
	raw  string
	none bool
}

// Ref names a palette slot.
func Ref(s Slot) *ColorRef { return &ColorRef{slot: s} }

// Raw is a literal "#rrggbb", "#rgb" or "0"–"255" color.
func Raw(s string) *ColorRef { return &ColorRef{raw: s} }

// None explicitly clears a color the layer below set.
func None() *ColorRef { return &ColorRef{none: true} }

var hexPattern = regexp.MustCompile(`^#([0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)

// ParseColorRef reads a config color value, naming the accepted forms on error.
func ParseColorRef(s string) (*ColorRef, error) {
	switch {
	case s == "none":
		return None(), nil
	case slices.Contains(Slots(), Slot(s)):
		return Ref(Slot(s)), nil
	case hexPattern.MatchString(s):
		return Raw(s), nil
	}
	if n, err := strconv.Atoi(s); err == nil && n >= 0 && n <= 255 && !strings.HasPrefix(s, "+") {
		return Raw(s), nil
	}
	names := make([]string, 0, len(Slots()))
	for _, sl := range Slots() {
		names = append(names, string(sl))
	}
	return nil, fmt.Errorf("invalid color %q: want one of %s, none, #rrggbb, or 0-255",
		s, strings.Join(names, " "))
}

// resolve turns the reference into a color for the palette and profile.
func (r *ColorRef) resolve(p Palette, prof colorprofile.Profile) color.Color {
	switch {
	case r.none:
		return lipgloss.NoColor{}
	case r.slot != "":
		return p.Get(r.slot).resolve(prof)
	case prof <= colorprofile.ASCII:
		return lipgloss.NoColor{}
	default:
		return prof.Convert(lipgloss.Color(r.raw))
	}
}

// Spec is one role's style as a layer states it. A nil field inherits from
// the layer below; a non-nil one replaces it, so an explicit false clears.
type Spec struct {
	Fg, Bg                         *ColorRef
	Bold, Faint, Italic, Underline *bool
}

// Over returns s layered on top of base.
func (s Spec) Over(base Spec) Spec {
	pick := func(top, bottom *bool) *bool {
		if top != nil {
			return top
		}
		return bottom
	}
	out := base
	if s.Fg != nil {
		out.Fg = s.Fg
	}
	if s.Bg != nil {
		out.Bg = s.Bg
	}
	out.Bold = pick(s.Bold, base.Bold)
	out.Faint = pick(s.Faint, base.Faint)
	out.Italic = pick(s.Italic, base.Italic)
	out.Underline = pick(s.Underline, base.Underline)
	return out
}

// style renders the spec into a lipgloss style for the palette and profile.
func (s Spec) style(p Palette, prof colorprofile.Profile) lipgloss.Style {
	st := lipgloss.NewStyle()
	if s.Fg != nil {
		st = st.Foreground(s.Fg.resolve(p, prof))
	}
	if s.Bg != nil {
		st = st.Background(s.Bg.resolve(p, prof))
	}
	if s.Bold != nil && *s.Bold {
		st = st.Bold(true)
	}
	if s.Faint != nil && *s.Faint {
		st = st.Faint(true)
	}
	if s.Italic != nil && *s.Italic {
		st = st.Italic(true)
	}
	if s.Underline != nil && *s.Underline {
		st = st.Underline(true)
	}
	return st
}

// on is a *bool true, for terse Spec literals.
var on = func() *bool { t := true; return &t }()
