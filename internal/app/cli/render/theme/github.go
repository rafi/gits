package theme

// GitHubDark — https://github.com/primer/primitives (functional/color/ansi-dark.json, base/color/dark.json; fgColor-default/muted, borderColor-default, prettylights-syntax-variable).
var GitHubDark = Theme{
	Name: "github-dark",
	Dark: true,
	Palette: Palette{
		Fg:      C("#e6edf3", 195, 7),
		Muted:   C("#848d97", 245, 8),
		Subtle:  C("#30363d", 237, 8),
		Red:     C("#ff7b72", 209, 1),
		Green:   C("#3fb950", 71, 2),
		Yellow:  C("#d29922", 172, 3),
		Blue:    C("#58a6ff", 75, 4),
		Magenta: C("#bc8cff", 141, 5),
		Cyan:    C("#39c5cf", 80, 6),
		Orange:  C("#ffa657", 215, 3),
		Accent:  C("#58a6ff", 75, 4),
	},
}

// GitHubLight — https://github.com/primer/primitives (functional/color/ansi-light.json, base/color/light.json; fgColor-default/muted, borderColor-default, fgColor-severe).
var GitHubLight = Theme{
	Name: "github-light",
	Dark: false,
	Palette: Palette{
		Fg:      C("#1f2328", 235, 0),
		Muted:   C("#636c76", 60, 8),
		Subtle:  C("#d0d7de", 188, 7),
		Red:     C("#cf222e", 160, 1),
		Green:   C("#1a7f37", 29, 2),
		Yellow:  C("#9a6700", 94, 3),
		Blue:    C("#0969da", 26, 4),
		Magenta: C("#8250df", 98, 5),
		Cyan:    C("#1b7c83", 30, 6),
		Orange:  C("#bc4c00", 130, 3),
		Accent:  C("#0969da", 26, 4),
	},
}
