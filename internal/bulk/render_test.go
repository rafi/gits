package bulk

import (
	"errors"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cli/clitest"
	"github.com/rafi/gits/internal/cli/config"
)

// TestRepoLine: a result line is the padded title followed by the body, or —
// when the result failed — by the styled error in its place.
func TestRepoLine(t *testing.T) {
	t.Parallel()

	theme := config.NewThemeDefault()
	title := theme.RepoTitle.SetString("api").Width(6)

	line := repoLine{Title: title, Body: "[main <- origin/main] ok"}
	if got := line.String(); !strings.Contains(got, "api") ||
		!strings.Contains(got, "[main <- origin/main] ok") {
		t.Errorf("String() = %q, want the title and the body", got)
	}

	line = repoLine{
		Title:      title,
		Body:       "never shown",
		Err:        errors.New("not cloned"),
		ErrorStyle: theme.Error,
	}
	got := line.String()
	if !strings.Contains(got, "not cloned") {
		t.Errorf("String() = %q, want the error", got)
	}
	if strings.Contains(got, "never shown") {
		t.Errorf("String() = %q, want the error to replace the body", got)
	}
}

// TestRenderHidesGroups: a body that reports a group as not shown takes the
// project's title with it, and the group's errors are still collected — which
// is what lets a filtered run drop a project whose every row was hidden
// without losing the failures underneath.
func TestRenderHidesGroups(t *testing.T) {
	t.Parallel()

	repo := func(name string) *Result[string] {
		return &Result[string]{Value: name, Err: errors.New(name + " failed")}
	}
	res := Results[string]{Groups: []Group[string]{
		{Project: domain.Project{Name: "shown"}, Results: []*Result[string]{repo("api")}},
		{Project: domain.Project{Name: "hidden"}, Results: []*Result[string]{repo("web")}},
	}}

	deps := clitest.New(t, nil)
	errs := Render(res, deps.RuntimeCLI, func(g Group[string]) (string, bool) {
		return g.Project.Name, g.Project.Name != "hidden"
	})

	got := deps.Result()
	if !strings.Contains(got, ":: shown") {
		t.Errorf("Result Output = %q, want the shown project", got)
	}
	if strings.Contains(got, "hidden") {
		t.Errorf("Result Output = %q, want the hidden project's title gone too", got)
	}
	if len(errs) != 2 {
		t.Fatalf("errors = %v, want both collected — a hidden row still failed", errs)
	}
}

// TestIndentMultiline verifies the first line is left untouched and every
// continuation line is indented and prefixed with "> ", including blank lines.
func TestIndentMultiline(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"single line", "repo  ~/path", "repo  ~/path"},
		{"empty", "", ""},
		{
			"multi line",
			"repo  ~/path err\nFetching rafi\nERROR: not found",
			"repo  ~/path err\n    > Fetching rafi\n    > ERROR: not found",
		},
		{
			"blank continuation line",
			"repo err\n\nmore",
			"repo err\n    > \n    > more",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := indentMultiline(tt.in); got != tt.want {
				t.Errorf("indentMultiline(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestTitleWidths: each project node keeps its own group width — equal to its
// widest repository title — and lookups survive the value copies the traversal
// makes, since nodes are identified by their shared Repos backing array.
func TestTitleWidths(t *testing.T) {
	t.Parallel()

	home := "/home/nobody"
	root := domain.Project{
		Name: "root",
		Repos: []domain.Repository{
			{Name: "long", Dir: "a-rather-long-repo-name"},
			{Name: "short", Dir: "short"},
		},
		SubProjects: []domain.Project{
			{Name: "sub", Repos: []domain.Repository{{Name: "s", Dir: "s"}}},
			{Name: "empty"},
		},
	}

	widths := newTitleWidths(root, home)
	rootCopy, subCopy := root, root.SubProjects[0]
	if got, want := widths.For(rootCopy), maxTitleWidth(root, home); got != want {
		t.Errorf("For(root) = %d, want %d", got, want)
	}
	if got, want := widths.For(subCopy), maxTitleWidth(root.SubProjects[0], home); got != want {
		t.Errorf("For(sub) = %d, want %d", got, want)
	}
	if got := widths.For(root.SubProjects[1]); got != 0 {
		t.Errorf("For(empty) = %d, want 0", got)
	}
	if got, want := widths.For(rootCopy), lipgloss.Width("a-rather-long-repo-name"); got != want {
		t.Errorf("For(root) = %d, want the widest title's rendered width %d", got, want)
	}
}
