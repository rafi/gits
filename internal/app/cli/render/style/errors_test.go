package style

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/rafi/gits/domain"
)

// TestRenderErrorsExcludesWarnings verifies that RenderErrors(_, true) counts
// only real errors, excluding domain.Warning — including a warning wrapped in
// another error (matched via [errors.As]).
func TestRenderErrorsExcludesWarnings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		errs    []error
		wantErr bool
	}{
		{"only warning", []error{domain.NewWarning("heads up")}, false},
		{"wrapped warning", []error{fmt.Errorf("ctx: %w", domain.NewWarning("heads up"))}, false},
		{"only real error", []error{fmt.Errorf("boom")}, true},
		{"mixed", []error{domain.NewWarning("warn"), fmt.Errorf("boom")}, true},
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
	err := RenderErrors(&diag, []error{fmt.Errorf("boom"), domain.NewWarning("meh")}, true)

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
	if err := RenderErrors(&empty, []error{domain.NewWarning("meh")}, true); err != nil {
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
	err := AbortOnRepoState(&diag, repo, NewThemeDefault().Error)

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
