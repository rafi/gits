// Package logging constructs the one [slog.Logger] gits uses. The logger is
// for debug tracing — page fetches, cache hits and misses, git's stderr —
// not for anything the user is meant to read: user-facing messages are
// Diagnostic Output, written as prose to deps.Err (see ADR-0004).
//
// Nothing here is global. The logger is built in cmd/gits and travels on
// types.Runtime, so a test can hand any component a logger of its own.
package logging

import (
	"io"
	"log/slog"
)

// New returns a logger writing to w with no timestamp, at debug level when
// verbose is set and at warn level otherwise — so a default run is silent
// unless something is worth saying.
func New(w io.Writer, verbose bool) *slog.Logger {
	level := slog.LevelWarn
	if verbose {
		level = slog.LevelDebug
	}
	return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{
		Level: level,
		// The process is short-lived and the records interleave with the
		// command's own output; a wall-clock stamp on each line is noise.
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if len(groups) == 0 && a.Key == slog.TimeKey {
				return slog.Attr{}
			}
			return a
		},
	}))
}

// Discard returns a logger that drops every record. It is the fallback for a
// component constructed without one, so a nil logger never panics deep in a
// call path.
func Discard() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

// Or returns logger, or a discarding logger when it is nil.
func Or(logger *slog.Logger) *slog.Logger {
	if logger == nil {
		return Discard()
	}
	return logger
}
