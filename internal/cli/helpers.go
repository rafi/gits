package cli

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cli/config"
	"github.com/rafi/gits/internal/types"
)

// The margins every Repository title is rendered with, so titles from
// different commands line up.
const (
	LeftMargin  = 2
	RightMargin = 2
)

// The sentinel errors a non-OK Repo State is reported as when the state
// carries no Reason of its own.
var (
	ErrNotRepository = fmt.Errorf("not a repository")
	ErrNotCloned     = fmt.Errorf("not cloned")
)

// StateError maps a non-OK repository state to its error: the Reason the
// state was classified for when it carries one, its sentinel otherwise. The
// bulk module's state guard reports a turned-back repository through it.
func StateError(repo domain.Repository) error {
	switch repo.State {
	case domain.RepoStateError:
		// The error state carries the Reason it was classified for, and that
		// reason is not always "this isn't a repository" — a readable clone
		// whose git command failed lands here too. Report what actually went
		// wrong; the sentinel is the fallback when nothing said.
		if repo.Reason != "" {
			return errors.New(repo.Reason)
		}
		return ErrNotRepository
	case domain.RepoStateNotCloned:
		return ErrNotCloned
	case domain.RepoStateUnknown, domain.RepoStateRemoteOnly, domain.RepoStateOK:
		// Not error states. A caller reaching here asked for the error of a
		// repository that has none; report the state itself rather than
		// inventing one.
		fallthrough
	default:
		return errors.New(string(repo.State))
	}
}

// AbortOnRepoState writes an error message as Diagnostic Output and aborts if
// the repository is in an error state. Used by single-repo/interactive
// callers; a Bulk Command declares the states it accepts instead, and the
// module renders the guard failure on the repository's own line.
//
// The message is terminated here, so a caller that writes a repository title
// ahead of it shares that line and appends no newline of its own.
func AbortOnRepoState(w io.Writer, repo domain.Repository, style lipgloss.Style) error {
	err := StateError(repo)
	lipgloss.Fprintln(w, style.Render(err.Error()))
	return RepoError(err, repo)
}

// RepoError wraps a repo failure as a *types.Warning (ErrorType) so it counts
// as a real error and matches uniformly via [errors.As].
func RepoError(err error, repo domain.Repository) error {
	return &types.Warning{
		Title:  repo.GetName(),
		Reason: err.Error(),
		Dir:    repo.AbsPath,
		Cause:  err,
	}
}

// IndentContinuation keeps the first line of s untouched and prefixes every
// continuation line with prefix. A single-line input is returned unchanged.
// The error epilogue and the bulk module's result lines each pass their own
// prefix; the pass over the string is the same one.
func IndentContinuation(s, prefix string) string {
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

// RenderErrors writes the run's error epilogue as Diagnostic Output and
// returns the error that decides the exit code — nil when nothing counted.
func RenderErrors(w io.Writer, errs []error, excludeWarnings bool) error {
	out := []string{}
	count := 0
	for _, err := range errs {
		if excludeWarnings && types.IsWarning(err) {
			continue
		}
		count++
		out = append(out, IndentContinuation(fmt.Sprintf("  - %s", err), "      > "))
	}
	if count < 1 {
		return nil
	}
	title := "error" + Plural(count)
	out = append([]string{"", fmt.Sprintf("%d %s:", count, title), ""}, out...)
	out = append(out, "")
	fmt.Fprintln(w, strings.Join(out, "\n"))
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
	if sourceName := getSourceType(project); sourceName != "" {
		title = fmt.Sprintf("%s %s", title, theme.Provider.Render(sourceName))
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

// Path returns a clean path with ~ for the home directory. The substitution
// is a directory match, not a string prefix: a sibling that merely shares the
// home directory's textual prefix (e.g. /Users/rafibar under /Users/rafi) is
// left untouched, and an empty homeDir never matches.
func Path(path, homeDir string) string {
	path = filepath.Clean(path)
	if homeDir == "" {
		return path
	}
	if path == homeDir {
		return "~"
	}
	if rest, cut := strings.CutPrefix(path, homeDir+string(filepath.Separator)); cut {
		return "~" + string(filepath.Separator) + rest
	}
	return path
}

// getSourceType returns the source type name of a project.
func getSourceType(p domain.Project) string {
	if p.Source == nil {
		return ""
	}
	return p.Source.Type
}
