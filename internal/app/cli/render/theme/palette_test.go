package theme

import (
	"image/color"
	"math"
	"regexp"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
)

var fullHex = regexp.MustCompile(`^#[0-9a-f]{6}$`)

// TestPalettesComplete: every named theme sets all 11 slots with a lowercase
// #rrggbb, an ANSI256 pick outside the terminal-defined 0–15, and an ANSI16
// pick in 0–15. The ansi theme is exempt by design.
func TestPalettesComplete(t *testing.T) {
	t.Parallel()
	for _, name := range Names() {
		if name == Ansi.Name {
			continue
		}
		th, _ := Lookup(name)
		for _, s := range Slots() {
			c := th.Palette.Get(s)
			if !fullHex.MatchString(c.True) {
				t.Errorf("%s.%s True = %q, want lowercase #rrggbb", name, s, c.True)
			}
			if c.ANSI256 < 16 {
				t.Errorf("%s.%s ANSI256 = %d, want 16–255", name, s, c.ANSI256)
			}
			if c.ANSI > 15 {
				t.Errorf("%s.%s ANSI = %d, want 0–15", name, s, c.ANSI)
			}
		}
	}
}

// TestANSI256PicksAreClose guards hand-picked 256-color fallbacks against
// typos: each must be within maxDeltaE (CIE76) of its hex, allowing some
// slack past colorprofile's own nearest pick.
func TestANSI256PicksAreClose(t *testing.T) {
	t.Parallel()
	const slack = 12.0
	for _, name := range Names() {
		if name == Ansi.Name {
			continue
		}
		th, _ := Lookup(name)
		for _, s := range Slots() {
			c := th.Palette.Get(s)
			hex := lipgloss.Color(c.True)
			auto := colorprofile.ANSI256.Convert(hex)
			picked := ansi.IndexedColor(c.ANSI256)
			if d, best := deltaE(hex, picked), deltaE(hex, auto); d > best+slack {
				t.Errorf("%s.%s ANSI256 %d is ΔE %.1f from %s; colorprofile's pick is ΔE %.1f",
					name, s, c.ANSI256, d, c.True, best)
			}
		}
	}
}

// TestContrast: text stays readable on the background the theme is for —
// Fg ≥ 4.5:1 and Muted ≥ 3:1 against black (dark) or white (light).
func TestContrast(t *testing.T) {
	t.Parallel()
	for _, name := range Names() {
		if name == Ansi.Name {
			continue
		}
		th, _ := Lookup(name)
		bg := 1.0 // white
		if th.Dark {
			bg = 0.0
		}
		for _, chk := range []struct {
			slot Slot
			min  float64
		}{{SlotFg, 4.5}, {SlotMuted, 3.0}} {
			l := luminance(lipgloss.Color(th.Palette.Get(chk.slot).True))
			hi, lo := math.Max(l, bg), math.Min(l, bg)
			if r := (hi + 0.05) / (lo + 0.05); r < chk.min {
				t.Errorf("%s.%s contrast %.2f:1, want ≥ %.1f:1", name, chk.slot, r, chk.min)
			}
		}
	}
}

// luminance is WCAG relative luminance.
func luminance(c color.Color) float64 {
	r, g, b, _ := c.RGBA()
	lin := func(v uint32) float64 {
		f := float64(v) / 0xffff
		if f <= 0.03928 {
			return f / 12.92
		}
		return math.Pow((f+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(r) + 0.7152*lin(g) + 0.0722*lin(b)
}

// deltaE is CIE76 distance in Lab space.
func deltaE(a, b color.Color) float64 {
	l1, a1, b1 := lab(a)
	l2, a2, b2 := lab(b)
	return math.Sqrt((l1-l2)*(l1-l2) + (a1-a2)*(a1-a2) + (b1-b2)*(b1-b2))
}

// lab converts sRGB to CIE Lab (D65).
func lab(c color.Color) (float64, float64, float64) {
	r, g, b, _ := c.RGBA()
	lin := func(v uint32) float64 {
		f := float64(v) / 0xffff
		if f <= 0.04045 {
			return f / 12.92
		}
		return math.Pow((f+0.055)/1.055, 2.4)
	}
	rl, gl, bl := lin(r), lin(g), lin(b)
	x := (0.4124*rl + 0.3576*gl + 0.1805*bl) / 0.95047
	y := 0.2126*rl + 0.7152*gl + 0.0722*bl
	z := (0.0193*rl + 0.1192*gl + 0.9505*bl) / 1.08883
	f := func(t float64) float64 {
		if t > 0.008856 {
			return math.Cbrt(t)
		}
		return 7.787*t + 16.0/116
	}
	fx, fy, fz := f(x), f(y), f(z)
	return 116*fy - 16, 500 * (fx - fy), 200 * (fy - fz)
}
