package theme

import (
	"fmt"
	"slices"
	"strings"
)

// all is every compiled-in theme. Order is irrelevant; Names sorts.
var all = []Theme{
	Ansi,
	Dracula,
	CatppuccinLatte, CatppuccinFrappe, CatppuccinMacchiato, CatppuccinMocha,
	TokyoNight, TokyoNightStorm, TokyoNightDay,
	Nord,
	GruvboxDark, GruvboxLight,
	SolarizedDark, SolarizedLight,
	OneDark, OneLight,
	RosePine, RosePineMoon, RosePineDawn,
	Kanagawa,
	EverforestDark, EverforestLight,
	GitHubDark, GitHubLight,
	MonokaiPro,
	AyuDark, AyuLight,
	MaterialPalenight,
	NightOwl,
	Nightfox, Dayfox,
}

// Names returns every theme name, sorted: the list completion, validation
// messages and `gits theme list` are all built from.
func Names() []string {
	names := make([]string, 0, len(all))
	for _, t := range all {
		names = append(names, t.Name)
	}
	slices.Sort(names)
	return names
}

// Lookup returns the theme called name, or an error listing the valid names.
func Lookup(name string) (Theme, error) {
	for _, t := range all {
		if t.Name == name {
			return t, nil
		}
	}
	return Theme{}, fmt.Errorf("unknown theme %q: must be one of %s",
		name, strings.Join(Names(), ", "))
}

// FlagValue is a pflag.Value binding --theme to a string, rejecting unknown
// names so a typo fails loudly instead of silently meaning ansi.
type FlagValue struct{ dst *string }

// NewFlagValue binds a theme flag to dst.
func NewFlagValue(dst *string) *FlagValue { return &FlagValue{dst: dst} }

// String returns the current value.
func (v *FlagValue) String() string {
	if v.dst == nil {
		return ""
	}
	return *v.dst
}

// Set validates and stores a theme name.
func (v *FlagValue) Set(s string) error {
	if _, err := Lookup(s); err != nil {
		return err
	}
	*v.dst = s
	return nil
}

// Type is the flag's value type name, shown in help.
func (v *FlagValue) Type() string { return "theme" }
