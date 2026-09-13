package format

import (
	"testing"

	"github.com/rafi/gits/domain"
)

// TestRepoRelPath: RepoRelPath is the single source for repo display paths,
// deriving one from the repository's configured Dir or its absolute path.
func TestRepoRelPath(t *testing.T) {
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
