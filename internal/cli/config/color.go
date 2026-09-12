// Package config reads the gits config file and turns its settings into the
// Theme the CLI renders with.
package config

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
