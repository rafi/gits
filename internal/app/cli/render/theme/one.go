package theme

// OneDark — https://github.com/atom/one-dark-syntax (styles/colors.less)
var OneDark = Theme{
	Name: "one-dark",
	Dark: true,
	Palette: Palette{
		Fg:      C("#abb2bf", 249, 7),
		Muted:   C("#5c6370", 241, 8),
		Subtle:  C("#3e4452", 238, 8),
		Red:     C("#e06c75", 246, 1),
		Green:   C("#98c379", 114, 2),
		Yellow:  C("#e5c07b", 180, 3),
		Blue:    C("#61afef", 75, 4),
		Magenta: C("#c678dd", 176, 5),
		Cyan:    C("#56b6c2", 73, 6),
		Orange:  C("#d19a66", 173, 3),
		Accent:  C("#61afef", 75, 4),
	},
}

// OneLight — https://github.com/atom/one-light-syntax (styles/colors.less)
var OneLight = Theme{
	Name: "one-light",
	Dark: false,
	Palette: Palette{
		Fg:      C("#383a42", 237, 0),
		Muted:   C("#696c77", 60, 8),
		Subtle:  C("#e5e5e6", 254, 7),
		Red:     C("#e45649", 167, 1),
		Green:   C("#50a14f", 71, 2),
		Yellow:  C("#c18401", 136, 3),
		Blue:    C("#4078f2", 69, 4),
		Magenta: C("#a626a4", 127, 5),
		Cyan:    C("#0184bc", 31, 6),
		Orange:  C("#986801", 94, 3),
		Accent:  C("#4078f2", 69, 4),
	},
}
