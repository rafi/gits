package theme

// Nord — https://www.nordtheme.com/docs/colors-and-palettes
// Muted is not one of the 16 official Nord colors: nord3 ("comments",
// #4c566a) falls short of the 3:1 contrast floor against a black
// background, and nord4 is already Fg, so this hand-picked tone between
// the two keeps Nord's neutral ramp while passing the contrast test.
var Nord = Theme{
	Name: "nord",
	Dark: true,
	Palette: Palette{
		Fg:      C("#d8dee9", 254, 7),
		Muted:   C("#7b88a1", 103, 8),
		Subtle:  C("#434c5e", 239, 8),
		Red:     C("#bf616a", 131, 1),
		Green:   C("#a3be8c", 144, 2),
		Yellow:  C("#ebcb8b", 222, 3),
		Blue:    C("#81a1c1", 109, 4),
		Magenta: C("#b48ead", 139, 5),
		Cyan:    C("#88c0d0", 110, 6),
		Orange:  C("#d08770", 173, 3),
		Accent:  C("#88c0d0", 110, 6),
	},
}
