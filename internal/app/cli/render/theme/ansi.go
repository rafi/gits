package theme

import (
	"github.com/charmbracelet/colorprofile"

	"github.com/rafi/gits/internal/app/cli/render/style"
)

// Ansi is the default theme: every slot is a terminal-palette index, so the
// terminal's own color scheme decides the shades. Its tweaks reproduce the
// pre-theme defaults exactly, including their few 256-color picks, so users
// who never choose a theme see no change.
var Ansi = Theme{
	Name: "ansi",
	Dark: true,
	Palette: Palette{
		Fg: A(15), Muted: A(8), Subtle: A(8),
		Red: A(1), Green: A(2), Yellow: A(3), Blue: A(4),
		Magenta: A(5), Cyan: A(6), Orange: A(3), Accent: A(5),
	},
	Tweaks: map[Role]Spec{
		RoleGitOutput:        {Fg: Raw("66")},
		RoleBranchName:       {Fg: Raw("68")},
		RoleDiff:             {Fg: Raw("140")},
		RoleStatusDim:        {Fg: None(), Faint: on},
		RoleStatusFooter:     {Fg: None(), Faint: on},
		RoleTableBorderStyle: {Fg: Raw("240")},
		RoleTableHeader:      {Fg: Raw("73")},
		RoleTableRowEven:     {Fg: Raw("249")},
		RoleTableRowOdd:      {Fg: Raw("247")},
		RoleProgressName:     {Fg: Raw("12")},
		RoleProgressPercent:  {Fg: Raw("14")},
		RoleProgressFill:     {Fg: Raw("10")},
		RoleProgressEmpty:    {Fg: Raw("240")},
		RoleProgressStats:    {Fg: Raw("240")},
	},
}

// DefaultStyles is the ansi theme at TrueColor: the fixed style.Theme tests
// across packages render with, standing in for whatever theme.Resolve would
// otherwise build for a real command run.
func DefaultStyles() style.Theme { return Build(Ansi, colorprofile.TrueColor, nil) }
