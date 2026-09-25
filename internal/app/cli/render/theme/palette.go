// Package theme defines the CLI's selectable color themes: a semantic palette
// per theme, one shared mapping from palette to style roles, and Build, which
// layers that mapping, per-theme tweaks and user overrides into a style.Theme
// resolved for the terminal's color profile.
package theme

import (
	"image/color"
	"strconv"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
)

// Color is one palette entry with a hand-picked value per color profile, so a
// 256-color terminal gets a chosen index rather than a computed approximation.
type Color struct {
	True    string // "#rrggbb"; empty for a terminal-palette color (see A)
	ANSI256 uint8
	ANSI    uint8 // 0–15
}

// C is a palette color: its TrueColor hex and its ANSI256 and ANSI16 picks.
func C(hex string, c256, c16 uint8) Color { return Color{True: hex, ANSI256: c256, ANSI: c16} }

// A is a terminal-palette color: the same ANSI index at every profile, so the
// terminal's own scheme decides the shade. The ansi theme is built from these.
func A(n uint8) Color { return Color{ANSI256: n, ANSI: n} }

// resolve returns the member of c's trio the profile calls for; NoTTY and
// ASCII resolve to no color.
func (c Color) resolve(p colorprofile.Profile) color.Color {
	truecolor := c.True
	if truecolor == "" {
		truecolor = strconv.Itoa(int(c.ANSI256))
	}
	return lipgloss.Complete(p)(
		lipgloss.Color(strconv.Itoa(int(c.ANSI))),
		lipgloss.Color(strconv.Itoa(int(c.ANSI256))),
		lipgloss.Color(truecolor),
	)
}

// Slot names one palette color. The string is the name users write in config.
type Slot string

const (
	SlotFg      Slot = "fg"
	SlotMuted   Slot = "muted"
	SlotSubtle  Slot = "subtle"
	SlotRed     Slot = "red"
	SlotGreen   Slot = "green"
	SlotYellow  Slot = "yellow"
	SlotBlue    Slot = "blue"
	SlotMagenta Slot = "magenta"
	SlotCyan    Slot = "cyan"
	SlotOrange  Slot = "orange"
	SlotAccent  Slot = "accent"
)

// Slots lists every palette slot in display order.
func Slots() []Slot {
	return []Slot{SlotFg, SlotMuted, SlotSubtle, SlotRed, SlotGreen, SlotYellow,
		SlotBlue, SlotMagenta, SlotCyan, SlotOrange, SlotAccent}
}

// Palette is a theme's semantic colors: text, metadata, rules, the eight hues
// and the theme's signature accent.
type Palette struct {
	Fg, Muted, Subtle        Color
	Red, Green, Yellow, Blue Color
	Magenta, Cyan, Orange    Color
	Accent                   Color
}

// Get returns the color in slot s.
func (p Palette) Get(s Slot) Color {
	switch s {
	case SlotFg:
		return p.Fg
	case SlotMuted:
		return p.Muted
	case SlotSubtle:
		return p.Subtle
	case SlotRed:
		return p.Red
	case SlotGreen:
		return p.Green
	case SlotYellow:
		return p.Yellow
	case SlotBlue:
		return p.Blue
	case SlotMagenta:
		return p.Magenta
	case SlotCyan:
		return p.Cyan
	case SlotOrange:
		return p.Orange
	default:
		// Reached only for SlotAccent: Slot values come exclusively from the
		// Slot constants above or from config already validated against
		// Slots() (see ParseColorRef), so this is never an unrecognized name
		// silently mapped to Accent.
		return p.Accent
	}
}
