package cli

import (
	"bytes"
	"fmt"
	"os"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cli/config"
	"github.com/rafi/gits/internal/types"
)

// TestIndentMultiline verifies the first line is left untouched and every
// continuation line is indented and prefixed with "> ", including blank lines.
func TestIndentMultiline(t *testing.T) {
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
			if got := IndentMultiline(tt.in); got != tt.want {
				t.Errorf("IndentMultiline(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestRenderErrorsExcludesWarnings verifies that RenderErrors(_, true) counts
// only real errors, excluding types.Warning — including a warning wrapped in
// another error (matched via errors.As).
func TestRenderErrorsExcludesWarnings(t *testing.T) {
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
			if got := RenderErrors(tt.errs, true); (got != nil) != tt.wantErr {
				t.Fatalf("RenderErrors(%v) err=%v, wantErr=%v", tt.errs, got, tt.wantErr)
			}
		})
	}
}

// TestRepoErrorIsPointerWarning verifies RepoError yields a *types.Warning so it
// matches uniformly via errors.As, and that it is an ErrorType (counts as a
// failure, not a warning).
func TestRepoErrorIsPointerWarning(t *testing.T) {
	repo := domain.Repository{Name: "acme", AbsPath: "/tmp/acme"}
	err := RepoError(fmt.Errorf("fetch failed"), repo)

	if RenderErrors([]error{err}, true) == nil {
		t.Fatal("RepoError should count as a real error, but was excluded")
	}
}

// TestTermWidth: non-terminal writers (buffers, /dev/null) report not-a-TTY
// with zero width, so callers can fall back to unbounded rendering.
func TestTermWidth(t *testing.T) {
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
// paths — RepoTitle renders it and GetMaxLen measures it, including
// ~-substituted home paths the old byte-length count overshot.
func TestRepoPathDerivation(t *testing.T) {
	home := "/home/nobody"
	project := domain.Project{
		Name:    "p",
		AbsPath: "/somewhere/else",
		Repos: []domain.Repository{
			{Name: "far", AbsPath: home + "/code/deeply/nested/long-repo-name"},
			{Name: "short", Dir: "short", AbsPath: "/somewhere/else/short"},
		},
	}
	theme := config.NewThemeDefault()

	if got := RepoRelPath(project, project.Repos[0], home); got != "~/code/deeply/nested/long-repo-name" {
		t.Errorf("RepoRelPath(home repo) = %q, want ~-substituted path", got)
	}
	if got := RepoRelPath(project, project.Repos[1], home); got != "short" {
		t.Errorf("RepoRelPath(dir repo) = %q, want short", got)
	}

	widest := 0
	for _, r := range project.Repos {
		widest = max(widest, lipgloss.Width(RepoTitle(r, project, home, theme).Value()))
	}
	if got := GetMaxLen(project, home); got != widest {
		t.Errorf("GetMaxLen = %d, want %d (rendered width of widest title)", got, widest)
	}
}

// TestTitleWidths: each project node keeps its own group width — equal to its
// GetMaxLen — and lookups survive the value copies the walker makes, since
// nodes are identified by their shared Repos backing array.
func TestTitleWidths(t *testing.T) {
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

	widths := NewTitleWidths(root, home)
	rootCopy, subCopy := root, root.SubProjects[0]
	if got, want := widths.For(rootCopy), GetMaxLen(root, home); got != want {
		t.Errorf("For(root) = %d, want %d", got, want)
	}
	if got, want := widths.For(subCopy), GetMaxLen(root.SubProjects[0], home); got != want {
		t.Errorf("For(sub) = %d, want %d", got, want)
	}
	if got := widths.For(root.SubProjects[1]); got != 0 {
		t.Errorf("For(empty) = %d, want 0", got)
	}
}
