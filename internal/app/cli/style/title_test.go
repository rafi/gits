package style

import (
	"regexp"
	"strings"
	"testing"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/app/format"
)

// TestRepoTitleRendersRelPath: RepoTitle shows exactly the display path
// format.RepoRelPath derives, including ~-substituted home paths the old
// byte-length count overshot.
func TestRepoTitleRendersRelPath(t *testing.T) {
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

	theme := NewThemeDefault()
	for _, repo := range project.Repos {
		want := format.RepoRelPath(project, repo, home)
		if got := RepoTitle(repo, project, home, theme).Value(); got != want {
			t.Errorf("RepoTitle(%q) = %q, want it rendered from %q", repo.Name, got, want)
		}
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

	theme := NewThemeDefault()

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
