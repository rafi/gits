package theme

import (
	"testing"

	"github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/colorprofile"
)

func TestGlamourFromPalette(t *testing.T) {
	t.Parallel()
	cfg := Glamour(Scheme{Theme: Dracula, Profile: colorprofile.TrueColor})
	if cfg.Heading.Color == nil || *cfg.Heading.Color != "#bd93f9" {
		t.Errorf("heading color = %v, want dracula accent", cfg.Heading.Color)
	}
	if cfg.Link.Color == nil || *cfg.Link.Color != "#bd93f9" { // dracula Blue slot is bd93f9
		t.Errorf("link color = %v, want the Blue slot", cfg.Link.Color)
	}
	low := Glamour(Scheme{Theme: Dracula, Profile: colorprofile.ANSI256})
	if low.Heading.Color == nil || *low.Heading.Color != "141" {
		t.Errorf("ANSI256 heading color = %v, want the palette's 256 pick", low.Heading.Color)
	}
}

func TestChromaStylesExist(t *testing.T) {
	t.Parallel()
	for _, name := range Names() {
		cs := chromaStyleName(name)
		if cs == "" {
			continue
		}
		if _, ok := styles.Registry[cs]; !ok {
			t.Errorf("theme %s maps to chroma style %q, which chroma does not register", name, cs)
		}
	}
}
