package logging_test

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/rafi/gits/internal/logging"
)

// TestNewLevels proves the verbosity contract: a default run traces nothing,
// and `-v` turns on debug records. Log records are not Diagnostic Output —
// nothing a user must read depends on this level.
func TestNewLevels(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		verbose   bool
		wantDebug bool
	}{
		{"quiet by default", false, false},
		{"debug when verbose", true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			l := logging.New(&buf, tt.verbose)
			l.Debug("tracing", "k", "v")
			l.Warn("noticed")

			got := buf.String()
			if hasDebug := strings.Contains(got, "tracing"); hasDebug != tt.wantDebug {
				t.Errorf("debug record present = %v, want %v (output %q)",
					hasDebug, tt.wantDebug, got)
			}
			if !strings.Contains(got, "noticed") {
				t.Errorf("warn record missing from %q", got)
			}
		})
	}
}

// TestNewOmitsTimestamp proves records carry no wall clock: they interleave
// with a command's own output, where a stamp per line is noise.
func TestNewOmitsTimestamp(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logging.New(&buf, true).Debug("tracing")

	if got := buf.String(); strings.Contains(got, "time=") {
		t.Errorf("record %q carries a timestamp, want none", got)
	}
}

// TestOrNeverReturnsNil proves the nil guard every component relies on: a
// client constructed without a logger traces to a discarding one instead of
// panicking deep in a call path.
func TestOrNeverReturnsNil(t *testing.T) {
	t.Parallel()

	l := logging.Or(nil)
	if l == nil {
		t.Fatal("Or(nil) = nil, want a discarding logger")
	}
	l.Debug("must not panic")

	var buf bytes.Buffer
	given := logging.New(&buf, true)
	if got := logging.Or(given); got != given {
		t.Error("Or(logger) returned a different logger, want the one passed in")
	}
}

// TestDiscardIsDisabled proves the fallback logger short-circuits: a
// discarding handler reports every level disabled, so building attributes for
// a dropped record costs nothing.
func TestDiscardIsDisabled(t *testing.T) {
	t.Parallel()

	if logging.Discard().Enabled(t.Context(), slog.LevelError) {
		t.Error("Discard() is enabled at error level, want every level disabled")
	}
}
