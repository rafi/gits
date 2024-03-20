package types

import (
	"fmt"
	"strings"
)

type Type int

const (
	ErrorType Type = iota
	WarningType
)

type Warning struct {
	Type   Type
	Title  string
	Reason string
	Dir    string
}

func NewWarning(reason string, args ...interface{}) error {
	return &Warning{
		Type:   WarningType,
		Reason: fmt.Sprintf(reason, args...),
	}
}

func NewError(reason string, args ...interface{}) error {
	return &Warning{
		Type:   ErrorType,
		Reason: fmt.Sprintf(reason, args...),
	}
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
