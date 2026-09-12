package cli

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"charm.land/lipgloss/v2"

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

// repoStateError maps a non-OK repository state to its sentinel error.
func repoStateError(repo domain.Repository) error {
	switch repo.State {
	case domain.RepoStateError:
		return ErrNotRepository
	case domain.RepoStateNoLocal:
		return ErrNotCloned
	default:
		return errors.New(string(repo.State))
	}
}

// AbortOnRepoState prints an error message and aborts if the repository is in
// an error state. Used by single-repo/interactive callers; the bulk walker uses
// RepoStateError to avoid writing to stdout from a RepoFunc.
func AbortOnRepoState(repo domain.Repository, style lipgloss.Style) error {
	err := repoStateError(repo)
	lipgloss.Print(style.Render(err.Error()))
	return RepoError(err, repo)
}

// RepoStateError returns the rendered state line and the wrapped error for a
// non-OK repository, without writing to stdout, so it is safe inside a
// walk.RepoFunc.
func RepoStateError(repo domain.Repository, style lipgloss.Style) (string, error) {
	err := repoStateError(repo)
	return style.Render(err.Error()), RepoError(err, repo)
}

// RepoStateWarning wraps a non-OK repository's state as a *types.Warning,
// rendering no line — for walk.RepoFunc callers that build their own result
// line and only need the error.
func RepoStateWarning(repo domain.Repository) error {
	return RepoError(repoStateError(repo), repo)
}

// RepoError wraps a repo failure as a *types.Warning (ErrorType) so it counts
// as a real error and matches uniformly via errors.As.
func RepoError(err error, repo domain.Repository) error {
	return &types.Warning{
		Title:  repo.GetName(),
		Reason: err.Error(),
		Dir:    repo.AbsPath,
		Cause:  err,
	}
}

// IndentMultiline reformats a rendered repo line so its continuation lines (any
// after the first) are indented and prefixed with "> ". This keeps multi-line
// git output and errors visually attached to their repo row instead of bleeding
// out to the left margin. A single-line input is returned unchanged.
func IndentMultiline(line string) string {
	return indentContinuation(line, "    > ")
}

// indentContinuation keeps the first line of s untouched and prefixes every
// continuation line with prefix. A single-line input is returned unchanged.
func indentContinuation(s, prefix string) string {
	head, rest, found := strings.Cut(s, "\n")
	if !found {
		return s
	}
	lines := strings.Split(rest, "\n")
	for i, l := range lines {
		lines[i] = prefix + l
	}
	return head + "\n" + strings.Join(lines, "\n")
}

func RenderErrors(errs []error, excludeWarnings bool) error {
	out := []string{}
	count := 0
	for _, err := range errs {
		if excludeWarnings && types.IsWarning(err) {
			continue
		}
		count++
		out = append(out, indentContinuation(fmt.Sprintf("  - %s", err), "      > "))
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
func RepoTitle(repo domain.Repository, basePath string, homeDir string, theme config.Theme) lipgloss.Style {
	repoPath := repo.Dir
	if repoPath == "" {
		repoPath = repo.AbsPath
	}
	repoPath = strings.TrimPrefix(repoPath, basePath+"/")
	repoPath = Path(repoPath, homeDir)
	return theme.RepoTitle.
		MarginLeft(LeftMargin).
		MarginRight(RightMargin).
		SetString(repoPath)
}

// PaddedRepoTitle is the common prologue of every walk.RepoFunc: the repo's
// title padded to the project's widest repo name so the result bodies align
// (AC-7).
func PaddedRepoTitle(repo domain.Repository, project domain.Project, deps types.RuntimeCLI) lipgloss.Style {
	return RepoTitle(repo, project.AbsPath, deps.HomeDir, deps.Theme).
		Width(GetMaxLen(project))
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
