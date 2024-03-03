package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cli/config"
)

const (
	LeftMargin  = 2
	RightMargin = 2
)

type Error struct {
	Message string
	Title   string
	Dir     string
}

// HandlerErrors prints a list of errors.
func HandlerErrors(list []Error) {
	errCount := len(list)
	if errCount > 0 {
		title := "error" + strings.Repeat("s", min(1, errCount))
		fmt.Printf("\n%d %s:\n", errCount, title)
		for _, err := range list {
			fmt.Printf("  - %s (%s): %s\n", err.Title, err.Dir, err.Message)
		}
	}
}

func GetTheme(themeSettings domain.Theme) (config.Theme, error) {
	theme := config.NewThemeDefault()
	if err := theme.ParseConfig(themeSettings); err != nil {
		return theme, err
	}
	return theme, nil
}

// AbortOnRepoState prints an error message and aborts if the repository is in
// an error state.
func AbortOnRepoState(repo domain.Repository, theme config.Theme) error {
	path := repo.AbsPath
	fmt.Printf(" %s", path)
	switch repo.State {
	case domain.RepoStateError:
		fmt.Printf(" %s", theme.Error.Render("Not a Git repository"))
	case domain.RepoStateNoLocal:
		fmt.Printf(" %s", theme.Error.Render("Not cloned"))
	}
	fmt.Println()
	return nil
}

// ProjectTitleWithBullet returns a formatted project title.
func ProjectTitleWithBullet(project domain.Project, theme config.Theme) string {
	return fmt.Sprintf(
		"%s %s",
		theme.Bullet.Render("::"),
		ProjectTitle(project, theme),
	)
}

// ProjectTitle returns a formatted project title.
func ProjectTitle(project domain.Project, theme config.Theme) string {
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
func ProjectTreeTitle(project domain.Project, homeDir string, theme config.Theme) string {
	title := theme.ProjectTitle.Render(project.Name)
	sourceName := getSourceType(project)
	if sourceName != "" {
		sourceName := theme.Provider.Render(sourceName)
		title = fmt.Sprintf("%s %s", title, sourceName)
	}
	projectPath := ""
	if project.AbsPath != "" {
		projectPath = Path(project.AbsPath, homeDir)
	}
	title = fmt.Sprintf("%s %s", title, theme.RepoPath.Render(projectPath))
	return title
}

// RepoTitle returns a formatted repository title.
func RepoTitle(project domain.Project, repo domain.Repository, homeDir string) lipgloss.Style {
	repoPath := repo.Dir
	if repoPath == "" {
		repoPath = repo.AbsPath
	}
	repoPath = strings.TrimPrefix(repoPath, project.AbsPath+"/")
	repoPath = Path(repoPath, homeDir)
	t := lipgloss.NewStyle()
	return t.SetString(repoPath)
}

// Path returns a clean path with ~ for home directory.
func Path(path, homeDir string) string {
	cut := false
	path = filepath.Clean(path)
	if path, cut = strings.CutPrefix(path, homeDir); cut {
		path = "~" + path
	}
	return path
}

// GetMaxLen returns length of the widest repo directory in a project.
func GetMaxLen(project domain.Project) int {
	maxLen := 0
	for _, repo := range project.Repos {
		repoPath := repo.Dir
		if repoPath == "" {
			repoPath = strings.TrimPrefix(repo.AbsPath, project.AbsPath+"/")
		}
		if i := len(repoPath); i > maxLen {
			maxLen = i
		}
	}
	if maxLen > 30 {
		maxLen = 30
	}
	return maxLen
}

// getSourceType returns the source type name of a project.
func getSourceType(p domain.Project) string {
	if p.Source == nil {
		return ""
	}
	return string(p.Source.Type)
}
