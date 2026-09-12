package cli

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cli/config"
	"github.com/rafi/gits/internal/types"
)

// TestRenderErrorsExcludesWarnings verifies that RenderErrors(_, true) counts
// only real errors, excluding types.Warning — including a warning wrapped in
// another error (matched via [errors.As]).
func TestRenderErrorsExcludesWarnings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		errs    []error
		wantErr bool
	}{
		{"only warning", []error{types.NewWarning("heads up")}, false},
		{"wrapped warning", []error{fmt.Errorf("ctx: %w", types.NewWarning("heads up"))}, false},
		{"only real error", []error{fmt.Errorf("boom")}, true},
		{"mixed", []error{types.NewWarning("warn"), fmt.Errorf("boom")}, true},
		{"empty", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := RenderErrors(io.Discard, tt.errs, true); (got != nil) != tt.wantErr {
				t.Fatalf("RenderErrors(%v) err=%v, wantErr=%v", tt.errs, got, tt.wantErr)
			}
		})
	}
}

// TestRenderErrorsWritesDiagnosticOutput proves the epilogue goes where the
// caller says — Diagnostic Output — and not to a process stream of its own
// choosing, so a run's outcome is assertable and never pollutes Result Output.
func TestRenderErrorsWritesDiagnosticOutput(t *testing.T) {
	t.Parallel()

	var diag bytes.Buffer
	err := RenderErrors(&diag, []error{fmt.Errorf("boom"), types.NewWarning("meh")}, true)

	if err == nil || err.Error() != "completed with errors" {
		t.Fatalf("RenderErrors() = %v, want the unchanged exit-code error", err)
	}
	got := diag.String()
	if !strings.Contains(got, "1 error:") || !strings.Contains(got, "boom") {
		t.Errorf("epilogue %q missing the counted error", got)
	}
	if strings.Contains(got, "meh") {
		t.Errorf("epilogue %q listed an excluded warning", got)
	}

	var empty bytes.Buffer
	if err := RenderErrors(&empty, []error{types.NewWarning("meh")}, true); err != nil {
		t.Errorf("RenderErrors(only warnings) = %v, want nil", err)
	}
	if empty.Len() != 0 {
		t.Errorf("nothing counted, yet wrote %q", empty.String())
	}
}

// TestAbortOnRepoStateTerminatesItsLine proves the abort message is written as
// Diagnostic Output and terminated exactly once, which is what lets every
// caller share the line with a title and append no newline of its own.
func TestAbortOnRepoStateTerminatesItsLine(t *testing.T) {
	t.Parallel()

	repo := domain.Repository{
		Name:   "acme",
		State:  domain.RepoStateError,
		Reason: "not a readable git repository",
	}

	var diag bytes.Buffer
	err := AbortOnRepoState(&diag, repo, config.NewThemeDefault().Error)

	if RenderErrors(io.Discard, []error{err}, true) == nil {
		t.Error("an aborted repo should count toward the exit code")
	}
	got := diag.String()
	if !strings.Contains(got, repo.Reason) {
		t.Errorf("abort message %q lost the repository's Reason", got)
	}
	if !strings.HasSuffix(got, "\n") {
		t.Errorf("abort message %q is not terminated", got)
	}
	if strings.HasSuffix(got, "\n\n") {
		t.Errorf("abort message %q is terminated twice", got)
	}
}

// TestRepoErrorIsPointerWarning verifies RepoError yields a *types.Warning so it
// matches uniformly via [errors.As], and that it is an ErrorType (counts as a
// failure, not a warning).
func TestRepoErrorIsPointerWarning(t *testing.T) {
	t.Parallel()

	repo := domain.Repository{Name: "acme", AbsPath: "/tmp/acme"}
	err := RepoError(fmt.Errorf("fetch failed"), repo)

	if RenderErrors(io.Discard, []error{err}, true) == nil {
		t.Fatal("RepoError should count as a real error, but was excluded")
	}
}

// TestTermWidth: non-terminal writers (buffers, /dev/null) report not-a-TTY
// with zero width, so callers can fall back to unbounded rendering.
func TestTermWidth(t *testing.T) {
	t.Parallel()

	if w, isTTY := TermWidth(&bytes.Buffer{}); w != 0 || isTTY {
		t.Errorf("TermWidth(buffer) = (%d, %v), want (0, false)", w, isTTY)
	}
	null, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer null.Close()
	if w, isTTY := TermWidth(null); w != 0 || isTTY {
		t.Errorf("TermWidth(devnull) = (%d, %v), want (0, false)", w, isTTY)
	}
}

// TestRepoPathDerivation: RepoRelPath is the single source for repo display
// paths — RepoTitle renders it, including ~-substituted home paths the old
// byte-length count overshot.
func TestRepoPathDerivation(t *testing.T) {
	t.Parallel()

	home := "/home/nobody"
	project := domain.Project{
		Name:    "p",
		AbsPath: "/somewhere/else",
		Repos: []domain.Repository{
			{Name: "far", AbsPath: home + "/code/deeply/nested/long-repo-name"},
			{Name: "short", Dir: "short", AbsPath: "/somewhere/else/short"},
		},
	}

	if got := RepoRelPath(project, project.Repos[0], home); got != "~/code/deeply/nested/long-repo-name" {
		t.Errorf("RepoRelPath(home repo) = %q, want ~-substituted path", got)
	}
	if got := RepoRelPath(project, project.Repos[1], home); got != "short" {
		t.Errorf("RepoRelPath(dir repo) = %q, want short", got)
	}

	theme := config.NewThemeDefault()
	for _, repo := range project.Repos {
		want := RepoRelPath(project, repo, home)
		if got := RepoTitle(repo, project, home, theme).Value(); got != want {
			t.Errorf("RepoTitle(%q) = %q, want it rendered from %q", repo.Name, got, want)
		}
	}
}

// TestPathHomeSubstitution pins that ~ is substituted on a directory match,
// not a bare string prefix: a sibling that shares the home directory's textual
// prefix must be left alone, and an empty homeDir must never match.
func TestPathHomeSubstitution(t *testing.T) {
	t.Parallel()

	const home = "/Users/rafi"
	tests := []struct {
		name string
		path string
		home string
		want string
	}{
		{"exact home", home, home, "~"},
		{"child of home", home + "/code/x", home, "~/code/x"},
		{"sibling sharing prefix", "/Users/rafibar/code/x", home, "/Users/rafibar/code/x"},
		{"unrelated path", "/opt/thing", home, "/opt/thing"},
		{"empty home leaves path", "/Users/rafi/code/x", "", "/Users/rafi/code/x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := Path(tt.path, tt.home); got != tt.want {
				t.Errorf("Path(%q, %q) = %q, want %q", tt.path, tt.home, got, tt.want)
			}
		})
	}
}

// TestRepoRelPathSiblingPrefix pins that RepoRelPath does not strip a project
// root that is only a textual prefix of the repository's path: it appends the
// separator, so a sibling directory sharing the prefix keeps its full path.
func TestRepoRelPathSiblingPrefix(t *testing.T) {
	t.Parallel()

	project := domain.Project{Name: "p", AbsPath: "/code/acme"}
	repo := domain.Repository{Name: "acme-web", AbsPath: "/code/acme-web/api"}

	if got := RepoRelPath(project, repo, "/home/nobody"); got != "/code/acme-web/api" {
		t.Errorf("RepoRelPath(sibling) = %q, want the full path kept", got)
	}
}

// TestRepoStateErrorSurfacesReason proves the Reason carried by an `error`
// repository reaches the user instead of being flattened into "not a
// repository". A repo whose git command failed — a missing git binary, say —
// is still a repository, and saying otherwise sends the reader looking in the
// wrong place.
func TestRepoStateErrorSurfacesReason(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		repo domain.Repository
		want string
	}{
		{
			"reason is surfaced verbatim",
			domain.Repository{
				State:  domain.RepoStateError,
				Reason: "unable to get remote URL: git executable not found in PATH",
			},
			"unable to get remote URL: git executable not found in PATH",
		},
		{
			"error without a reason falls back to the sentinel",
			domain.Repository{State: domain.RepoStateError},
			ErrNotRepository.Error(),
		},
		{
			"not-cloned is unchanged",
			domain.Repository{State: domain.RepoStateNotCloned, Reason: "ignored"},
			ErrNotCloned.Error(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := StateError(tt.repo).Error(); got != tt.want {
				t.Errorf("StateError() = %q, want %q", got, tt.want)
			}
		})
	}
}

// stripANSI removes SGR escape sequences, so an assertion can be made on the
// text a user sees rather than on the styling lipgloss wraps it in.
func stripANSI(s string) string {
	return ansiPattern.ReplaceAllString(s, "")
}

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// TestProjectTitles pins the three project title renderers. The theme used is
// the plain one lipgloss falls back to without a color profile, so the
// assertions are on the composition — name, source, description, path — and
// not on escape codes.
func TestProjectTitles(t *testing.T) {
	t.Parallel()

	theme := config.NewThemeDefault()

	t.Run("title carries source and description", func(t *testing.T) {
		t.Parallel()

		p := domain.Project{
			Name:   "acme",
			Desc:   "all the things",
			Source: &domain.ProviderSource{Type: "github"},
		}
		got := ProjectTitle(p, theme)
		for _, want := range []string{"acme", "[github]", "(all the things)"} {
			if !strings.Contains(got, want) {
				t.Errorf("ProjectTitle() = %q, want it to contain %q", got, want)
			}
		}
	})

	t.Run("title omits an absent source and description", func(t *testing.T) {
		t.Parallel()

		got := stripANSI(ProjectTitle(domain.Project{Name: "acme"}, theme))
		if got != "acme" {
			t.Errorf("ProjectTitle() = %q, want exactly %q with no source or description", got, "acme")
		}
	})

	t.Run("bullet prefixes the title", func(t *testing.T) {
		t.Parallel()

		p := domain.Project{Name: "acme"}
		got := ProjectTitleWithBullet(p, theme)
		if !strings.Contains(got, "::") {
			t.Errorf("ProjectTitleWithBullet() = %q, want it to contain the bullet", got)
		}
		if !strings.Contains(got, ProjectTitle(p, theme)) {
			t.Errorf("ProjectTitleWithBullet() = %q, want it to contain the plain title", got)
		}
	})

	t.Run("tree title renders the path with ~", func(t *testing.T) {
		t.Parallel()

		p := domain.Project{
			Name:    "acme",
			AbsPath: "/home/rafi/code/acme",
			Source:  &domain.ProviderSource{Type: "gitlab"},
		}
		got := ProjectTreeTitle(p, "/home/rafi", theme)
		for _, want := range []string{"acme", "gitlab", "~/code/acme"} {
			if !strings.Contains(got, want) {
				t.Errorf("ProjectTreeTitle() = %q, want it to contain %q", got, want)
			}
		}
	})

	t.Run("tree title of a project with no local home", func(t *testing.T) {
		t.Parallel()

		p := domain.Project{Name: "acme"}
		got := ProjectTreeTitle(p, "/home/rafi", theme)
		if !strings.Contains(got, "acme") {
			t.Errorf("ProjectTreeTitle() = %q, want it to contain %q", got, "acme")
		}
	})
}

func TestPlural(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		n    int
		want string
	}{{0, "s"}, {1, ""}, {2, "s"}} {
		if got := Plural(tc.n); got != tc.want {
			t.Errorf("Plural(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}
