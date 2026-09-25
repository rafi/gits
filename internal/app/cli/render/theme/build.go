package theme

import (
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"

	"github.com/rafi/gits/internal/app/cli/render/style"
)

// Theme is one selectable theme: its palette, whether it is designed for a
// dark background, and any role adjustments its palette needs.
type Theme struct {
	Name    string
	Dark    bool
	Palette Palette
	Tweaks  map[Role]Spec
}

// Build resolves t for profile p into the styles commands render with,
// layering the shared mapping, then t.Tweaks, then overrides, field by field.
// Overrides must already be validated (see ParseStyles).
func Build(t Theme, p colorprofile.Profile, overrides map[Role]Spec) style.Theme {
	out := style.Theme{TableBorder: lipgloss.NormalBorder()}
	for _, r := range Roles() {
		spec := baseMapping[r]
		if tw, ok := t.Tweaks[r]; ok {
			spec = tw.Over(spec)
		}
		if ov, ok := overrides[r]; ok {
			spec = ov.Over(spec)
		}
		*roleFields[r](&out) = spec.style(t.Palette, p)
	}
	return out
}
