package config

import (
	"github.com/knadh/koanf/v2"
)

const (
	themeKey       = settingsKey + ".theme"
	themeNameKey   = themeKey + ".name"
	themeStylesKey = themeKey + ".styles"
)

// legacyThemeNotice is the one deprecation warning a pre-0.3 flat theme block
// produces, showing the shape to move to.
const legacyThemeNotice = "settings.theme: role keys directly under `theme:` are " +
	"deprecated; move them under `theme.styles:` (and `color:` to `fg:`), e.g.\n" +
	"  theme:\n    name: ansi\n    styles:\n      repoTitle: { fg: \"4\", bold: true }"

// normalizeTheme rewrites settings.theme in the koanf tree into the one shape
// domain.ThemeSettings decodes: a string name becomes {dark, light}, and
// legacy role keys move under styles with color renamed fg and the layout
// keys (align, width) dropped. It works on the tree rather than in a custom
// unmarshaler because koanf decodes through mapstructure, for every file
// format alike. It reports whether any legacy key was moved.
func normalizeTheme(k *koanf.Koanf) bool {
	if name, ok := k.Get(themeNameKey).(string); ok {
		_ = k.Set(themeNameKey, map[string]any{"dark": name, "light": name})
	}

	raw, ok := k.Get(themeKey).(map[string]any)
	if !ok {
		return false
	}
	legacy := false
	for key, val := range raw {
		if key == "name" || key == "styles" {
			continue
		}
		spec, ok := val.(map[string]any)
		if !ok {
			continue // not a style; left for the unknown-key warning
		}
		legacy = true
		moved := map[string]any{}
		for field, v := range spec {
			switch field {
			case "color":
				moved["fg"] = v
			case "align", "width":
			default:
				moved[field] = v
			}
		}
		k.Delete(themeKey + "." + key)
		_ = k.Set(themeStylesKey+"."+key, moved)
	}
	return legacy
}
