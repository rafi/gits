package theme

// RosePine — https://github.com/rose-pine/palette (palette.json, main).
var RosePine = Theme{
	Name: "rose-pine",
	Dark: true,
	Palette: Palette{
		Fg:      C("#e0def4", 189, 7),
		Muted:   C("#6e6a86", 60, 8),
		Subtle:  C("#403d52", 238, 8),
		Red:     C("#eb6f92", 204, 1),
		Green:   C("#31748f", 66, 2),
		Yellow:  C("#f6c177", 216, 3),
		Blue:    C("#9ccfd8", 152, 4),
		Magenta: C("#c4a7e7", 182, 5),
		Cyan:    C("#ebbcba", 217, 6),
		Orange:  C("#ebbcba", 217, 3),
		Accent:  C("#c4a7e7", 182, 5),
	},
}

// RosePineMoon — https://github.com/rose-pine/palette (palette.json, moon).
var RosePineMoon = Theme{
	Name: "rose-pine-moon",
	Dark: true,
	Palette: Palette{
		Fg:      C("#e0def4", 189, 7),
		Muted:   C("#6e6a86", 60, 8),
		Subtle:  C("#44415a", 239, 8),
		Red:     C("#eb6f92", 204, 1),
		Green:   C("#3e8fb0", 67, 2),
		Yellow:  C("#f6c177", 216, 3),
		Blue:    C("#9ccfd8", 152, 4),
		Magenta: C("#c4a7e7", 182, 5),
		Cyan:    C("#ea9a97", 174, 6),
		Orange:  C("#ea9a97", 174, 3),
		Accent:  C("#c4a7e7", 182, 5),
	},
}

// RosePineDawn — https://github.com/rose-pine/palette (palette.json, dawn)
// Muted uses Rosé Pine's own "subtle" hex (#797593), not its "muted" one
// (#9893a5): dawn is a light theme, and "muted" falls short of the 3:1
// contrast floor against a light background, the same problem nord.go's
// Muted works around.
var RosePineDawn = Theme{
	Name: "rose-pine-dawn",
	Dark: false,
	Palette: Palette{
		Fg:      C("#464261", 239, 0),
		Muted:   C("#797593", 244, 8),
		Subtle:  C("#dfdad9", 253, 7),
		Red:     C("#b4637a", 132, 1),
		Green:   C("#286983", 24, 2),
		Yellow:  C("#ea9d34", 179, 3),
		Blue:    C("#56949f", 67, 4),
		Magenta: C("#907aa9", 103, 5),
		Cyan:    C("#d7827e", 174, 6),
		Orange:  C("#d7827e", 174, 3),
		Accent:  C("#907aa9", 103, 5),
	},
}
