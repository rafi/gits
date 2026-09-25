package theme

// EverforestDark — https://github.com/sainnhe/everforest (autoload/everforest.vim, medium)
var EverforestDark = Theme{
	Name: "everforest-dark",
	Dark: true,
	Palette: Palette{
		Fg:      C("#d3c6aa", 187, 7),
		Muted:   C("#859289", 245, 8),
		Subtle:  C("#475258", 239, 8),
		Red:     C("#e67e80", 174, 1),
		Green:   C("#a7c080", 144, 2),
		Yellow:  C("#dbbc7f", 180, 3),
		Blue:    C("#7fbbb3", 109, 4),
		Magenta: C("#d699b6", 175, 5),
		Cyan:    C("#83c092", 108, 6),
		Orange:  C("#e69875", 174, 3),
		Accent:  C("#a7c080", 144, 2),
	},
}

// EverforestLight — https://github.com/sainnhe/everforest (autoload/everforest.vim, medium)
var EverforestLight = Theme{
	Name: "everforest-light",
	Dark: false,
	Palette: Palette{
		Fg:      C("#5c6a72", 242, 0),
		Muted:   C("#829181", 245, 8),
		Subtle:  C("#e0dcc7", 253, 7),
		Red:     C("#f85552", 203, 1),
		Green:   C("#8da101", 106, 2),
		Yellow:  C("#dfa000", 178, 3),
		Blue:    C("#3a94c5", 68, 4),
		Magenta: C("#df69ba", 169, 5),
		Cyan:    C("#35a77c", 72, 6),
		Orange:  C("#f57d26", 208, 3),
		Accent:  C("#8da101", 106, 2),
	},
}
