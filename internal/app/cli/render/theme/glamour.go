package theme

import (
	"fmt"
	"image/color"
	"strconv"

	"charm.land/glamour/v2/ansi"
	"charm.land/lipgloss/v2"
	xansi "github.com/charmbracelet/x/ansi"
)

// chromaStyles maps themes to the chroma syntax style of the same scheme;
// themes without one get a code-block palette derived from their own colors.
var chromaStyles = map[string]string{
	"dracula":              "dracula",
	"catppuccin-latte":     "catppuccin-latte",
	"catppuccin-frappe":    "catppuccin-frappe",
	"catppuccin-macchiato": "catppuccin-macchiato",
	"catppuccin-mocha":     "catppuccin-mocha",
	"tokyo-night":          "tokyonight-night",
	"tokyo-night-storm":    "tokyonight-storm",
	"tokyo-night-day":      "tokyonight-day",
	"nord":                 "nord",
	"gruvbox-dark":         "gruvbox",
	"gruvbox-light":        "gruvbox-light",
	"solarized-dark":       "solarized-dark",
	"solarized-light":      "solarized-light",
	"one-dark":             "onedark",
	"rose-pine":            "rose-pine",
	"rose-pine-moon":       "rose-pine-moon",
	"rose-pine-dawn":       "rose-pine-dawn",
	"github-dark":          "github-dark",
	"github-light":         "github",
	"monokai-pro":          "monokai",
}

func chromaStyleName(themeName string) string { return chromaStyles[themeName] }

// colorString renders a resolved color the way glamour's style config expects
// it: an ANSI index for indexed colors, #rrggbb otherwise, "" for none. A
// NoTTY/ASCII profile resolves every slot to lipgloss.NoColor, whose RGBA()
// reports full alpha (it means "don't color this", not "black transparent"),
// so it needs its own case rather than falling through to the RGBA branch.
func colorString(c color.Color) string {
	switch v := c.(type) {
	case nil:
		return ""
	case lipgloss.NoColor:
		return ""
	case xansi.BasicColor:
		return strconv.Itoa(int(v))
	case xansi.IndexedColor:
		return strconv.Itoa(int(v))
	}
	r, g, b, a := c.RGBA()
	if a == 0 {
		return ""
	}
	return fmt.Sprintf("#%02x%02x%02x", r>>8, g>>8, b>>8)
}

// Glamour returns a glamour style for the README preview built from the
// scheme's palette, following the structure of glamour's own dracula style.
func Glamour(s Scheme) ansi.StyleConfig {
	c := func(slot Slot) *string {
		v := colorString(s.Color(slot))
		if v == "" {
			return nil
		}
		return &v
	}
	yes := true
	margin := uint(2)
	cfg := ansi.StyleConfig{
		Document: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{BlockPrefix: "\n", BlockSuffix: "\n", Color: c(SlotFg)},
			Margin:         &margin,
		},
		BlockQuote: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{Color: c(SlotMuted), Italic: &yes},
			Indent:         &margin,
		},
		List: ansi.StyleList{LevelIndent: margin},
		Heading: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{BlockSuffix: "\n", Color: c(SlotAccent), Bold: &yes},
		},
		H1:             ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "# "}},
		H2:             ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "## "}},
		H3:             ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "### "}},
		H4:             ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "#### "}},
		H5:             ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "##### "}},
		H6:             ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "###### "}},
		Strikethrough:  ansi.StylePrimitive{CrossedOut: &yes},
		Emph:           ansi.StylePrimitive{Color: c(SlotYellow), Italic: &yes},
		Strong:         ansi.StylePrimitive{Color: c(SlotOrange), Bold: &yes},
		HorizontalRule: ansi.StylePrimitive{Color: c(SlotMuted), Format: "\n--------\n"},
		Item:           ansi.StylePrimitive{BlockPrefix: "• "},
		Enumeration:    ansi.StylePrimitive{BlockPrefix: ". ", Color: c(SlotCyan)},
		Task:           ansi.StyleTask{Ticked: "[✓] ", Unticked: "[ ] "},
		Link:           ansi.StylePrimitive{Color: c(SlotBlue), Underline: &yes},
		LinkText:       ansi.StylePrimitive{Color: c(SlotMagenta)},
		Image:          ansi.StylePrimitive{Color: c(SlotBlue), Underline: &yes},
		ImageText:      ansi.StylePrimitive{Color: c(SlotMagenta), Format: "Image: {{.text}} →"},
		Code:           ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: c(SlotOrange)}},
		CodeBlock: ansi.StyleCodeBlock{
			StyleBlock: ansi.StyleBlock{
				StylePrimitive: ansi.StylePrimitive{Color: c(SlotFg)},
				Margin:         &margin,
			},
		},
		DefinitionDescription: ansi.StylePrimitive{BlockPrefix: "\n🠶 "},
	}
	if name := chromaStyleName(s.Theme.Name); name != "" {
		cfg.CodeBlock.Theme = name
	} else {
		cfg.CodeBlock.Chroma = &ansi.Chroma{
			Text:                ansi.StylePrimitive{Color: c(SlotFg)},
			Error:               ansi.StylePrimitive{Color: c(SlotRed)},
			Comment:             ansi.StylePrimitive{Color: c(SlotMuted)},
			CommentPreproc:      ansi.StylePrimitive{Color: c(SlotMagenta)},
			Keyword:             ansi.StylePrimitive{Color: c(SlotMagenta)},
			KeywordReserved:     ansi.StylePrimitive{Color: c(SlotMagenta)},
			KeywordNamespace:    ansi.StylePrimitive{Color: c(SlotMagenta)},
			KeywordType:         ansi.StylePrimitive{Color: c(SlotCyan)},
			Operator:            ansi.StylePrimitive{Color: c(SlotCyan)},
			Punctuation:         ansi.StylePrimitive{Color: c(SlotFg)},
			Name:                ansi.StylePrimitive{Color: c(SlotFg)},
			NameBuiltin:         ansi.StylePrimitive{Color: c(SlotCyan)},
			NameTag:             ansi.StylePrimitive{Color: c(SlotMagenta)},
			NameAttribute:       ansi.StylePrimitive{Color: c(SlotGreen)},
			NameClass:           ansi.StylePrimitive{Color: c(SlotYellow)},
			NameConstant:        ansi.StylePrimitive{Color: c(SlotOrange)},
			NameDecorator:       ansi.StylePrimitive{Color: c(SlotGreen)},
			NameFunction:        ansi.StylePrimitive{Color: c(SlotBlue)},
			LiteralNumber:       ansi.StylePrimitive{Color: c(SlotOrange)},
			LiteralString:       ansi.StylePrimitive{Color: c(SlotGreen)},
			LiteralStringEscape: ansi.StylePrimitive{Color: c(SlotMagenta)},
			GenericDeleted:      ansi.StylePrimitive{Color: c(SlotRed)},
			GenericEmph:         ansi.StylePrimitive{Color: c(SlotYellow), Italic: &yes},
			GenericInserted:     ansi.StylePrimitive{Color: c(SlotGreen)},
			GenericStrong:       ansi.StylePrimitive{Color: c(SlotOrange), Bold: &yes},
			GenericSubheading:   ansi.StylePrimitive{Color: c(SlotAccent)},
		}
	}
	return cfg
}
