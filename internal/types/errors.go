// Package types holds the runtime dependencies every command is handed, and
// the warning envelope commands report failures through.
package types

import (
	"errors"
	"fmt"
	"strings"
)

// Type separates a real failure from a downgradeable warning.
type Type int

// ErrorType counts toward the exit code; WarningType does not.
const (
	ErrorType Type = iota
	WarningType
)

// Warning is the envelope every command reports a per-Repository failure
// through, carrying the Repository it concerns alongside the cause. Its Type
// decides whether it counts toward the exit code.
//
// The name is the domain's, not the Go convention's: this is not always a
// failure, so naming it WarningError would misstate what it carries.
//
//nolint:errname // deliberate — see above.
type Warning struct {
	Type   Type
	Title  string
	Reason string
	Dir    string
	// Cause is the wrapped underlying error, if any, kept so the chain
	// stays visible to errors.Is/As.
	Cause error
}

// NewWarning formats a downgradeable warning, keeping the last error in args
// as its cause so the chain stays visible.
func NewWarning(reason string, args ...any) error {
	return &Warning{
		Type:   WarningType,
		Reason: fmt.Sprintf(reason, args...),
		Cause:  lastError(args),
	}
}

// IsWarning reports whether err is (or wraps) a *Warning of WarningType,
// i.e. a downgradeable warning rather than a real failure.
func IsWarning(err error) bool {
	var w *Warning
	return errors.As(err, &w) && w.Type == WarningType
}

// lastError returns the last error among format arguments, so warnings built
// with "%s"-formatted causes still wrap them.
func lastError(args []any) error {
	var cause error
	for _, a := range args {
		if err, ok := a.(error); ok {
			cause = err
		}
	}
	return cause
}

func (e Warning) Unwrap() error {
	return e.Cause
}

func (e Warning) Error() string {
	msg := ""
	if e.Title != "" {
		msg += " " + e.Title
		if e.Dir == "" {
			msg += ": "
		}
	}
	if e.Dir != "" {
		msg += fmt.Sprintf(" (%s)", e.Dir)
		msg += ": "
	}
	msg += e.Reason
	return strings.TrimSpace(msg)
}
