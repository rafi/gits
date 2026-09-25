package theme

// SolarizedDark — https://github.com/altercation/solarized
var SolarizedDark = Theme{
	Name: "solarized-dark",
	Dark: true,
	Palette: Palette{
		Fg:      C("#839496", 246, 7),
		Muted:   C("#657b83", 66, 8),
		Subtle:  C("#073642", 23, 8),
		Red:     C("#dc322f", 166, 1),
		Green:   C("#859900", 100, 2),
		Yellow:  C("#b58900", 136, 3),
		Blue:    C("#268bd2", 32, 4),
		Magenta: C("#d33682", 168, 5),
		Cyan:    C("#2aa198", 36, 6),
		Orange:  C("#cb4b16", 166, 3),
		Accent:  C("#6c71c4", 62, 5),
	},
}

// SolarizedLight — https://github.com/altercation/solarized
var SolarizedLight = Theme{
	Name: "solarized-light",
	Dark: false,
	Palette: Palette{
		Fg:      C("#586e75", 60, 0),
		Muted:   C("#839496", 246, 8),
		Subtle:  C("#eee8d5", 254, 7),
		Red:     C("#dc322f", 166, 1),
		Green:   C("#859900", 100, 2),
		Yellow:  C("#b58900", 136, 3),
		Blue:    C("#268bd2", 32, 4),
		Magenta: C("#d33682", 168, 5),
		Cyan:    C("#2aa198", 36, 6),
		Orange:  C("#cb4b16", 166, 3),
		Accent:  C("#6c71c4", 62, 5),
	},
}
