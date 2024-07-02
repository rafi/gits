package cli

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cli/config"
	"github.com/rafi/gits/internal/types"
)

const (
	LeftMargin  = 2
	RightMargin = 2
)

var (
	ErrNotRepository = fmt.Errorf("not a repository")
	ErrNotCloned     = fmt.Errorf("not cloned")
)

func GetTheme(themeSettings domain.Theme) (config.Theme, error) {
	theme := config.NewThemeDefault()
	if err := theme.ParseConfig(themeSettings); err != nil {
		return theme, err
	}
	return theme, nil
}

// AbortOnRepoState prints an error message and aborts if the repository is in
// an error state.
func AbortOnRepoState(repo domain.Repository, style lipgloss.Style) error {
	var err error
	switch repo.State {
	case domain.RepoStateError:
		err = ErrNotRepository
	case domain.RepoStateNoLocal:
		err = ErrNotCloned
	default:
		err = errors.New(string(repo.State))
	}
	fmt.Print(style.Render(err.Error()))
	return RepoError(err, repo)
}

func RepoError(err error, repo domain.Repository) types.Warning {
	return types.Warning{
		Title:  repo.GetName(),
		Reason: err.Error(),
		Dir:    repo.AbsPath,
	}
}

func RenderErrors(errs []error, excludeWarnings bool) error {
	out := []string{}
	count := 0
	for _, err := range errs {
		if excludeWarnings {
			// nolint:errorlint
			if e, ok := err.(*types.Warning); ok && e.Type == types.WarningType {
				continue
			}
		}
		count++
		out = append(out, fmt.Sprintf("  - %s", err))
	}
	if count < 1 {
		return nil
	}
	title := "error" + strings.Repeat("s", min(1, count-1))
	out = append([]string{"", fmt.Sprintf("%d %s:", count, title), ""}, out...)
	out = append(out, "")
	fmt.Println(strings.Join(out, "\n"))
	return errors.New("completed with errors")
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
func RepoTitle(project domain.Project, repo domain.Repository, homeDir string, theme config.Theme) lipgloss.Style {
	repoPath := repo.Dir
	if repoPath == "" {
		repoPath = repo.AbsPath
	}
	repoPath = strings.TrimPrefix(repoPath, project.AbsPath+"/")
	repoPath = Path(repoPath, homeDir)
	return theme.RepoTitle.Copy().
		MarginLeft(LeftMargin).
		MarginRight(RightMargin).
		SetString(repoPath)
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
	return maxLen
}

// getSourceType returns the source type name of a project.
func getSourceType(p domain.Project) string {
	if p.Source == nil {
		return ""
	}
	return p.Source.Type
}
