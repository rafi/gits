package theme

// GruvboxDark — https://github.com/morhetz/gruvbox (colors/gruvbox.vim)
var GruvboxDark = Theme{
	Name: "gruvbox-dark",
	Dark: true,
	Palette: Palette{
		Fg:      C("#ebdbb2", 223, 7),
		Muted:   C("#928374", 102, 8),
		Subtle:  C("#504945", 239, 8),
		Red:     C("#fb4934", 203, 1),
		Green:   C("#b8bb26", 142, 2),
		Yellow:  C("#fabd2f", 214, 3),
		Blue:    C("#83a598", 108, 4),
		Magenta: C("#d3869b", 174, 5),
		Cyan:    C("#8ec07c", 108, 6),
		Orange:  C("#fe8019", 208, 3),
		Accent:  C("#fe8019", 208, 3),
	},
}

// GruvboxLight — https://github.com/morhetz/gruvbox (colors/gruvbox.vim)
var GruvboxLight = Theme{
	Name: "gruvbox-light",
	Dark: false,
	Palette: Palette{
		Fg:      C("#3c3836", 237, 0),
		Muted:   C("#7c6f64", 95, 8),
		Subtle:  C("#d5c4a1", 187, 7),
		Red:     C("#9d0006", 124, 1),
		Green:   C("#79740e", 100, 2),
		Yellow:  C("#b57614", 136, 3),
		Blue:    C("#076678", 24, 4),
		Magenta: C("#8f3f71", 95, 5),
		Cyan:    C("#427b58", 65, 6),
		Orange:  C("#af3a03", 130, 3),
		Accent:  C("#af3a03", 130, 3),
	},
}
