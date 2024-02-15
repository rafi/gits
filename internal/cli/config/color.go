package config

type colorOption int

var ColorOptionDefault = ColorOptionAuto.String()

const (
	ColorOptionAuto colorOption = iota
	ColorOptionAlways
	ColorOptionNever
)

func (c colorOption) String() string {
	return [...]string{"auto", "always", "never"}[c]
}
