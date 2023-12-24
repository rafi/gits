package config

type colorOption int

var ColorOptionDefault = colorOptionAuto.String()

const (
	colorOptionAuto colorOption = iota
	colorOptionAlways
	colorOptionNever
)

func (c colorOption) String() string {
	return [...]string{"auto", "always", "never"}[c]
}
