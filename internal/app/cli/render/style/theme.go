package style

import (
	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
)

// Theme is the resolved render style of every element the CLI draws, built by
// theme.Build.
type Theme struct {
	// General
	Normal        lipgloss.Style
	PreviewHeader lipgloss.Style
	Bullet        lipgloss.Style

	// Project
	ProjectTitle lipgloss.Style
	Provider     lipgloss.Style
	Desc         lipgloss.Style

	// Repository
	RepoTitle lipgloss.Style
	RepoPath  lipgloss.Style
	GitOutput lipgloss.Style

	// Branch
	BranchName      lipgloss.Style
	BranchCurrent   lipgloss.Style
	BranchIndicator lipgloss.Style
	RemoteName      lipgloss.Style

	// Tag
	TagIndicator lipgloss.Style

	// Status
	Diff    lipgloss.Style
	Error   lipgloss.Style
	Warning lipgloss.Style

	// Status table
	StatusHeader    lipgloss.Style
	StatusFlag      lipgloss.Style
	StatusStaged    lipgloss.Style
	StatusUnstaged  lipgloss.Style
	StatusUntracked lipgloss.Style
	StatusAhead     lipgloss.Style
	StatusBehind    lipgloss.Style
	StatusDiverged  lipgloss.Style
	StatusGone      lipgloss.Style
	StatusAdded     lipgloss.Style
	StatusDeleted   lipgloss.Style
	StatusDim       lipgloss.Style
	StatusFooter    lipgloss.Style
	StatusAgeFresh  lipgloss.Style
	StatusAgeRecent lipgloss.Style
	StatusAgeStale  lipgloss.Style

	// List table
	TableBorder      lipgloss.Border
	TableBorderStyle lipgloss.Style
	TableHeader      lipgloss.Style
	TableRowEven     lipgloss.Style
	TableRowOdd      lipgloss.Style

	// Chart
	ChartDates lipgloss.Style

	// Progress reporter
	ProgressName    lipgloss.Style
	ProgressPercent lipgloss.Style
	ProgressFill    lipgloss.Style
	ProgressEmpty   lipgloss.Style
	ProgressStats   lipgloss.Style
	ProgressSpinner lipgloss.Style
}

// TableRowStyle returns the style for a table row, striping data rows and
// setting the header apart.
func (t *Theme) TableRowStyle(row, _ int) lipgloss.Style {
	var s lipgloss.Style
	switch {
	// v2 tables index the header as HeaderRow (-1) and data rows from 0, so the
	// striping is offset by one vs. v1 (where the header was row 0).
	case row == table.HeaderRow:
		s = t.TableHeader
	case row%2 == 0:
		s = t.TableRowOdd
	default:
		s = t.TableRowEven
	}
	s = s.Margin(0, 1)
	return s
}
