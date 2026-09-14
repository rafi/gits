package style

import (
	"fmt"

	"charm.land/lipgloss/v2"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/format"
)

// The margins every Repository title is rendered with, so titles from
// different commands line up.
const (
	LeftMargin  = 2
	RightMargin = 2
)

// ProjectTitleWithBullet returns a formatted project title.
func ProjectTitleWithBullet(project domain.Project, theme Theme) string {
	return fmt.Sprintf(
		"%s %s",
		theme.Bullet.Render("::"),
		ProjectTitle(project, theme),
	)
}

// ProjectTitle returns a formatted project title.
func ProjectTitle(project domain.Project, theme Theme) string {
	sourceName := getSourceType(project)
	if sourceName != "" {
		sourceName = theme.Provider.Render(" [" + sourceName + "]")
	}
	projectDesc := project.Desc
	if projectDesc != "" {
		projectDesc = theme.Desc.Render(" (" + projectDesc + ")")
	}

	return fmt.Sprintf(
		"%s%s%s",
		theme.ProjectTitle.Render(project.Name),
		sourceName,
		projectDesc,
	)
}

// ProjectTreeTitle returns a formatted project title for tree display.
func ProjectTreeTitle(project domain.Project, homeDir string, theme Theme) string {
	title := theme.ProjectTitle.Render(project.Name)
	if sourceName := getSourceType(project); sourceName != "" {
		title = fmt.Sprintf("%s %s", title, theme.Provider.Render(sourceName))
	}
	projectPath := ""
	if project.AbsPath != "" {
		projectPath = format.Path(project.AbsPath, homeDir)
	}
	title = fmt.Sprintf("%s %s", title, theme.RepoPath.Render(projectPath))
	return title
}

// RepoTitle returns a formatted repository title.
func RepoTitle(repo domain.Repository, project domain.Project, homeDir string, theme Theme) lipgloss.Style {
	return theme.RepoTitle.
		MarginLeft(LeftMargin).
		MarginRight(RightMargin).
		SetString(format.RepoRelPath(project, repo, homeDir))
}

// getSourceType returns the source type name of a project.
func getSourceType(p domain.Project) string {
	if p.Source == nil {
		return ""
	}
	return p.Source.Type
}
