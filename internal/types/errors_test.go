package types

import (
	"errors"
	"fmt"
	"io"
	"testing"
)

// TestWarningUnwrap proves a Warning built around another error keeps the
// chain intact for [errors.Is]/[errors.As] instead of severing it at format time.
func TestWarningUnwrap(t *testing.T) {
	t.Parallel()

	err := NewWarning("unable to load project: %s", io.EOF)
	if !errors.Is(err, io.EOF) {
		t.Errorf("errors.Is(NewWarning(..., io.EOF), io.EOF) = false, want true")
	}

	plain := NewWarning("no repository selected")
	if errors.Unwrap(plain) != nil {
		t.Errorf("Unwrap(plain warning) = %v, want nil", errors.Unwrap(plain))
	}
}

// TestIsWarning covers the shared warning predicate: only genuine
// WarningType values qualify — wrapped ones included — while plain errors
// and ErrorType values do not.
func TestIsWarning(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"warning", NewWarning("skipped"), true},
		{"error type", &Warning{Type: ErrorType, Reason: "failed"}, false},
		{"plain error", io.EOF, false},
		{"wrapped warning", fmt.Errorf("outer: %w", NewWarning("inner")), true},
	}
	for _, tc := range cases {
		if got := IsWarning(tc.err); got != tc.want {
			t.Errorf("IsWarning(%s) = %v, want %v", tc.name, got, tc.want)
		}
	}
}
