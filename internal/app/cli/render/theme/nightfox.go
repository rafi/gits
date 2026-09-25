package theme

// Nightfox — https://github.com/EdenEast/nightfox.nvim (lua/nightfox/palette/nightfox.lua)
var Nightfox = Theme{
	Name: "nightfox",
	Dark: true,
	Palette: Palette{
		Fg:      C("#cdcecf", 252, 7),
		Muted:   C("#738091", 244, 8),
		Subtle:  C("#39506d", 239, 8),
		Red:     C("#c94f6d", 167, 1),
		Green:   C("#81b29a", 108, 2),
		Yellow:  C("#dbc074", 180, 3),
		Blue:    C("#719cd6", 74, 4),
		Magenta: C("#9d79d6", 140, 5),
		Cyan:    C("#63cdcf", 80, 6),
		Orange:  C("#f4a261", 215, 3),
		Accent:  C("#719cd6", 74, 4),
	},
}

// Dayfox — https://github.com/EdenEast/nightfox.nvim (lua/nightfox/palette/dayfox.lua)
var Dayfox = Theme{
	Name: "dayfox",
	Dark: false,
	Palette: Palette{
		Fg:      C("#3d2b5a", 53, 0),
		Muted:   C("#837a72", 101, 8),
		Subtle:  C("#e7d2be", 187, 7),
		Red:     C("#a5222f", 124, 1),
		Green:   C("#396847", 59, 2),
		Yellow:  C("#ac5402", 130, 3),
		Blue:    C("#2848a9", 25, 4),
		Magenta: C("#6e33ce", 62, 5),
		Cyan:    C("#287980", 30, 6),
		Orange:  C("#955f61", 95, 3),
		Accent:  C("#2848a9", 25, 4),
	},
}
