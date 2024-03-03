package config

import (
	"fmt"
	"reflect"

	"github.com/charmbracelet/lipgloss"

	"github.com/rafi/gits/domain"
)

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
	Modified  lipgloss.Style
	Untracked lipgloss.Style
	Diff      lipgloss.Style
	Error     lipgloss.Style

	// List table
	TableBorder      lipgloss.Border
	TableBorderStyle lipgloss.Style
	TableHeader      lipgloss.Style
	TableRowEven     lipgloss.Style
	TableRowOdd      lipgloss.Style

	// Chart
	ChartDates lipgloss.Style
}

func (t *Theme) ParseConfig(cfg domain.Theme) (err error) {
	v := reflect.ValueOf(cfg)
	vp := reflect.ValueOf(t)
	typeOfS := v.Type()

	for i := 0; i < v.NumField(); i++ {
		name := typeOfS.Field(i).Name
		if v.Field(i).Type().Name() == "Style" {
			val := v.Field(i).Interface().(domain.Style)
			if val.Color == "" && val.Align == "" && val.Width == 0 {
				continue
			}
			style, err := parseThemeStyle(val)
			if err != nil {
				return err
			}
			vp.Elem().FieldByName(name).Set(reflect.ValueOf(style))
		}
	}
	return nil
}

func parseThemeStyle(style domain.Style) (lipgloss.Style, error) {
	s := lipgloss.NewStyle()
	if style.Color != "" {
		s = s.Foreground(lipgloss.Color(style.Color))
	}
	if style.Width > 0 {
		s = s.Width(style.Width)
	}
	switch style.Align {
	case "right":
		s = s.Align(lipgloss.Right)
	case "left":
		s = s.Align(lipgloss.Left)
	case "center":
		s = s.Align(lipgloss.Center)
	case "":
	default:
		return s, fmt.Errorf("invalid align value: %s", style.Align)
	}
	return s, nil
}

func NewThemeDefault() Theme {
	theme := Theme{
		// General
		Normal:        lipgloss.NewStyle(),
		PreviewHeader: lipgloss.NewStyle(),
		Bullet:        lipgloss.NewStyle().Foreground(lipgloss.Color("4")),

		// Project
		ProjectTitle: lipgloss.NewStyle().Foreground(lipgloss.Color("5")).Bold(true),
		Provider:     lipgloss.NewStyle().Foreground(lipgloss.Color("8")),
		Desc:         lipgloss.NewStyle().Foreground(lipgloss.Color("15")),

		// Repository
		RepoTitle: lipgloss.NewStyle().Foreground(lipgloss.Color("4")).Bold(true),
		RepoPath:  lipgloss.NewStyle().Foreground(lipgloss.Color("15")),
		GitOutput: lipgloss.NewStyle().Foreground(lipgloss.Color("66")),

		// Branch
		BranchName:      lipgloss.NewStyle().Foreground(lipgloss.Color("68")),
		BranchCurrent:   lipgloss.NewStyle().Foreground(lipgloss.Color("4")),
		BranchIndicator: lipgloss.NewStyle().Foreground(lipgloss.Color("4")).Bold(true),
		RemoteName:      lipgloss.NewStyle().Foreground(lipgloss.Color("1")),

		// Tag
		TagIndicator: lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true),

		// Status
		Modified:  lipgloss.NewStyle().Foreground(lipgloss.Color("169")).Width(3).Align(lipgloss.Right),
		Untracked: lipgloss.NewStyle().Foreground(lipgloss.Color("75")).Width(3).Align(lipgloss.Right),
		Diff:      lipgloss.NewStyle().Foreground(lipgloss.Color("140")).Align(lipgloss.Right),
		Error:     lipgloss.NewStyle().Foreground(lipgloss.Color("1")),

		// List table
		TableBorder:      lipgloss.NormalBorder(),
		TableBorderStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
		TableHeader:      lipgloss.NewStyle().Foreground(lipgloss.Color("73")),
		TableRowEven:     lipgloss.NewStyle().Foreground(lipgloss.Color("249")),
		TableRowOdd:      lipgloss.NewStyle().Foreground(lipgloss.Color("247")),

		// Chart
		ChartDates: lipgloss.NewStyle().Foreground(lipgloss.Color("8")),
	}
	return theme
}

func (t Theme) TableRowStyle(row, _ int) lipgloss.Style {
	var s lipgloss.Style
	switch {
	case row == 0:
		s = t.TableHeader
	case row%2 == 0:
		s = t.TableRowEven
	default:
		s = t.TableRowOdd
	}
	s.Copy().Margin(0, 1)
	return s
}
