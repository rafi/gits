package theme

// AyuDark — https://github.com/ayu-theme/ayu-colors (themes/dark.yaml).
var AyuDark = Theme{
	Name: "ayu-dark",
	Dark: true,
	Palette: Palette{
		Fg:      C("#bfbdb6", 250, 7),
		Muted:   C("#6c7380", 66, 8),
		Subtle:  C("#2d3640", 23, 8),
		Red:     C("#f07178", 204, 1),
		Green:   C("#aad94c", 149, 2),
		Yellow:  C("#e6b450", 179, 3),
		Blue:    C("#59c2ff", 75, 4),
		Magenta: C("#d2a6ff", 183, 5),
		Cyan:    C("#95e6cb", 116, 6),
		Orange:  C("#ff8f40", 209, 3),
		Accent:  C("#e6b450", 179, 3),
	},
}

// AyuLight — https://github.com/ayu-theme/ayu-colors (themes/light.yaml).
var AyuLight = Theme{
	Name: "ayu-light",
	Dark: false,
	Palette: Palette{
		Fg:      C("#5c6166", 59, 0),
		Muted:   C("#787b80", 244, 8),
		Subtle:  C("#d8d8d7", 188, 7),
		Red:     C("#f07171", 203, 1),
		Green:   C("#86b300", 106, 2),
		Yellow:  C("#f2ae49", 215, 3),
		Blue:    C("#399ee6", 74, 4),
		Magenta: C("#a37acc", 140, 5),
		Cyan:    C("#4cbf99", 72, 6),
		Orange:  C("#fa8d3e", 209, 3),
		Accent:  C("#ffaa33", 215, 3),
	},
}
