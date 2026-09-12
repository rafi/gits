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
// an error state. Used by single-repo/interactive callers; walk.RepoFunc
// callers use RepoStateWarning to avoid writing to stdout.
func AbortOnRepoState(repo domain.Repository, style lipgloss.Style) error {
	err := repoStateError(repo)
	lipgloss.Print(style.Render(err.Error()))
	return RepoError(err, repo)
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
	title := "error" + Plural(count)
	out = append([]string{"", fmt.Sprintf("%d %s:", count, title), ""}, out...)
	out = append(out, "")
	fmt.Println(strings.Join(out, "\n"))
	return errors.New("completed with errors")
}

// Plural returns the "s" suffix for a count-aware label.
func Plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
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

// RepoRelPath returns a repository's display path: its configured Dir or its
// absolute path made relative to the project root, with ~ for the home
// directory. Every title and width computation derives from this one place.
func RepoRelPath(project domain.Project, repo domain.Repository, homeDir string) string {
	repoPath := repo.Dir
	if repoPath == "" {
		repoPath = repo.AbsPath
	}
	repoPath = strings.TrimPrefix(repoPath, project.AbsPath+"/")
	return Path(repoPath, homeDir)
}

// RepoTitle returns a formatted repository title.
func RepoTitle(repo domain.Repository, project domain.Project, homeDir string, theme config.Theme) lipgloss.Style {
	return theme.RepoTitle.
		MarginLeft(LeftMargin).
		MarginRight(RightMargin).
		SetString(RepoRelPath(project, repo, homeDir))
}

// PaddedRepoTitle is the common prologue of every walk.RepoFunc: the repo's
// title padded to width — the project's widest repo name, precomputed once per
// walk via NewTitleWidths — so the result bodies align (AC-7).
func PaddedRepoTitle(repo domain.Repository, project domain.Project, width int, deps types.RuntimeCLI) lipgloss.Style {
	return RepoTitle(repo, project, deps.HomeDir, deps.Theme).Width(width)
}

// TitleWidths holds the widest repo title of every project node in a tree,
// measured once up front so walk.RepoFunc handlers — called once per repo,
// concurrently — don't remeasure the whole project for each repo. Nodes are
// identified by their Repos backing array, the one part of a project value
// that stays shared as the walker copies project nodes around.
type TitleWidths map[*domain.Repository]int

// NewTitleWidths measures every project node of root, root itself included,
// matching the walker's traversal so each group keeps its own padding.
func NewTitleWidths(root domain.Project, homeDir string) TitleWidths {
	widths := TitleWidths{}
	var visit func(p domain.Project)
	visit = func(p domain.Project) {
		if len(p.Repos) > 0 {
			widths[&p.Repos[0]] = GetMaxLen(p, homeDir)
		}
		for _, sub := range p.SubProjects {
			visit(sub)
		}
	}
	visit(root)
	return widths
}

// For returns project's precomputed title width; a project without repos has
// no titles to pad and yields zero.
func (tw TitleWidths) For(project domain.Project) int {
	if len(project.Repos) == 0 {
		return 0
	}
	return tw[&project.Repos[0]]
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

// GetMaxLen returns the rendered width of the widest repo display path in a
// project, measured on the same derivation the titles render.
func GetMaxLen(project domain.Project, homeDir string) int {
	maxLen := 0
	for _, repo := range project.Repos {
		maxLen = max(maxLen, lipgloss.Width(RepoRelPath(project, repo, homeDir)))
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
