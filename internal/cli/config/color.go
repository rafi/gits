// Package config reads the gits config file and turns its settings into the
// Theme the CLI renders with.
package config

import (
	"fmt"
	"slices"
)

type colorOption int

// ColorOptionDefault is the value of the --color flag when unset.
var ColorOptionDefault = ColorOptionAuto.String()

// The values accepted by the --color flag: colorize when the destination is
// a terminal, always, or never.
const (
	ColorOptionAuto colorOption = iota
	ColorOptionAlways
	ColorOptionNever
)

func (c colorOption) String() string {
	return [...]string{"auto", "always", "never"}[c]
}

// colorChoices are the three accepted --color values, in the order they are
// listed to the user on a rejected value.
var colorChoices = []string{
	ColorOptionAuto.String(),
	ColorOptionAlways.String(),
	ColorOptionNever.String(),
}

// ColorValue is a pflag.Value binding the --color flag to a string, rejecting
// anything but auto|always|never so a typo fails loudly instead of silently
// meaning auto.
type ColorValue struct {
	dst *string
}

// NewColorValue binds a color flag to dst, seeding it with the default.
func NewColorValue(dst *string) *ColorValue {
	*dst = ColorOptionDefault
	return &ColorValue{dst: dst}
}

// String returns the current value.
func (c *ColorValue) String() string {
	if c.dst == nil {
		return ColorOptionDefault
	}
	return *c.dst
}

// Set validates and stores a color value, listing the accepted ones on error.
func (c *ColorValue) Set(v string) error {
	if !slices.Contains(colorChoices, v) {
		return fmt.Errorf("invalid color %q: must be one of auto, always, never", v)
	}
	*c.dst = v
	return nil
}

// Type is the flag's value type name, shown in help.
func (c *ColorValue) Type() string { return "color" }
