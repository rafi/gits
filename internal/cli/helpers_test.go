package cli

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
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
// matches uniformly via errors.As, and that it is an ErrorType (counts as a
// failure, not a warning).
func TestRepoErrorIsPointerWarning(t *testing.T) {
	repo := domain.Repository{Name: "acme", AbsPath: "/tmp/acme"}
	err := RepoError(fmt.Errorf("fetch failed"), repo)

	if RenderErrors(io.Discard, []error{err}, true) == nil {
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

// TestRepoStateErrorSurfacesReason proves the Reason carried by an `error`
// repository reaches the user instead of being flattened into "not a
// repository". A repo whose git command failed — a missing git binary, say —
// is still a repository, and saying otherwise sends the reader looking in the
// wrong place.
func TestRepoStateErrorSurfacesReason(t *testing.T) {
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
			if got := stateError(tt.repo).Error(); got != tt.want {
				t.Errorf("stateError() = %q, want %q", got, tt.want)
			}
		})
	}
}
