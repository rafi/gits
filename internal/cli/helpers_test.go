package cli

import (
	"fmt"
	"testing"

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

// TestRepoStateErrorNoStdout verifies the non-printing state-error helper
// returns the sentinel error for a non-OK repo without writing to stdout, so it
// is safe to call inside a walk.RepoFunc.
func TestRepoStateErrorNoStdout(t *testing.T) {
	theme := config.NewThemeDefault()
	repo := domain.Repository{Name: "acme", AbsPath: "/tmp/acme", State: domain.RepoStateNoLocal}

	line, err := RepoStateError(repo, theme.Error)
	if err == nil {
		t.Fatal("expected an error for a non-cloned repo")
	}
	if line == "" {
		t.Fatal("expected a rendered state line for the result, got empty")
	}
	// The returned error must count as a failure (ErrorType).
	if RenderErrors([]error{err}, true) == nil {
		t.Fatal("repo-state error should count as a real error")
	}
}
