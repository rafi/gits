package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cli/types"
)

type Error struct {
	Message string
	Title   string
	Dir     string
}

// HandlerErrors prints a list of errors.
func HandlerErrors(list []Error) {
	if len(list) > 0 {
		fmt.Print("\nErrors:\n")
		for _, err := range list {
			fmt.Printf("  - %s (%s): %s\n", err.Title, err.Dir, err.Message)
		}
	}
}

// AbortOnRepoState prints an error message and aborts if the repository is in
// an error state.
func AbortOnRepoState(repo domain.Repository, theme types.Theme) error {
	path := repo.AbsPath
	fmt.Printf(" %s", path)
	switch repo.State {
	case domain.RepoStateError:
		fmt.Printf(" %s", theme.Error.Render("Not a Git repository"))
	case domain.RepoStateNoLocal:
		fmt.Printf(" %s", theme.Error.Render("Not cloned"))
	}
	fmt.Printf("\n")
	return nil
}

// ProjectTitleWithBullet returns a formatted project title.
func ProjectTitleWithBullet(project domain.Project, theme types.Theme) string {
	return fmt.Sprintf(
		"%s %s",
		theme.Bullet.Render("::"),
		ProjectTitle(project, theme),
	)
}

// ProjectTitle returns a formatted project title.
func ProjectTitle(project domain.Project, theme types.Theme) string {
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
func ProjectTreeTitle(project domain.Project, homeDir string, theme types.Theme) string {
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
