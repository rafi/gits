package theme

// TokyoNight — https://github.com/folke/tokyonight.nvim (colors/night.lua, colors/storm.lua)
var TokyoNight = Theme{
	Name: "tokyo-night",
	Dark: true,
	Palette: Palette{
		Fg:      C("#c0caf5", 153, 7),
		Muted:   C("#565f89", 60, 8),
		Subtle:  C("#3b4261", 239, 8),
		Red:     C("#f7768e", 210, 1),
		Green:   C("#9ece6a", 149, 2),
		Yellow:  C("#e0af68", 179, 3),
		Blue:    C("#7aa2f7", 111, 4),
		Magenta: C("#bb9af7", 141, 5),
		Cyan:    C("#7dcfff", 117, 6),
		Orange:  C("#ff9e64", 215, 3),
		Accent:  C("#bb9af7", 141, 5),
	},
}

// TokyoNightStorm — https://github.com/folke/tokyonight.nvim (colors/storm.lua)
var TokyoNightStorm = Theme{
	Name: "tokyo-night-storm",
	Dark: true,
	Palette: Palette{
		Fg:      C("#c0caf5", 153, 7),
		Muted:   C("#565f89", 60, 8),
		Subtle:  C("#3b4261", 239, 8),
		Red:     C("#f7768e", 210, 1),
		Green:   C("#9ece6a", 149, 2),
		Yellow:  C("#e0af68", 179, 3),
		Blue:    C("#7aa2f7", 111, 4),
		Magenta: C("#bb9af7", 141, 5),
		Cyan:    C("#7dcfff", 117, 6),
		Orange:  C("#ff9e64", 215, 3),
		Accent:  C("#7aa2f7", 111, 4),
	},
}

// TokyoNightDay — https://github.com/folke/tokyonight.nvim (colors/storm.lua inverted via util.invert; cross-checked against extras/prism/tokyonight_day.js and extras/alacritty/tokyonight_day.toml)
var TokyoNightDay = Theme{
	Name: "tokyo-night-day",
	Dark: false,
	Palette: Palette{
		Fg:      C("#3760bf", 61, 0),
		Muted:   C("#848cb5", 103, 8),
		Subtle:  C("#a8aecb", 146, 7),
		Red:     C("#f52a65", 197, 1),
		Green:   C("#587539", 65, 2),
		Yellow:  C("#8c6c3e", 95, 3),
		Blue:    C("#2e7de9", 32, 4),
		Magenta: C("#9854f1", 99, 5),
		Cyan:    C("#007197", 24, 6),
		Orange:  C("#b15c00", 130, 3),
		Accent:  C("#9854f1", 99, 5),
	},
}
