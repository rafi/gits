package config

import (
	"fmt"
	"sort"
	"strings"

	"github.com/go-viper/mapstructure/v2"
	"github.com/knadh/koanf/v2"
)

// settingsKey is the one top-level key that is not a project.
const settingsKey = "settings"

// deprecatedProjectsKey is the retired top-level `projects:` wrapper. Convert
// reports it by name with instructions, so its contents must not also be
// listed as unknown keys: everything nested under it looks unknown precisely
// because the wrapper is the thing that is wrong.
const deprecatedProjectsKey = "projects"

// unknownKeys reports the config keys that matched no field of the structs
// they were unmarshalled into — a misspelled `pth:` where `path:` was meant.
//
// koanf ignores such a key silently, which is how a project could end up with
// no path and no message. mapstructure records them for us: giving the decoder
// a Metadata sink makes it list every key it did not consume, with the full
// path to it, including list elements and nested sub-projects. That is why
// this is a second decode rather than a reflection walk over the struct tags —
// the decoder already knows the answer, and asking it cannot drift from what
// the real unmarshal accepts.
//
// Note the keys bind case-insensitively, so `cachettl` matches `cacheTTL` and
// is not reported here. The check catches misspellings, not miscapitalizations.
func unknownKeys(k *koanf.Koanf, projects any, settings any) []string {
	// The projects pass unmarshals the whole document into a project map, so
	// every settings key comes back as unused — `settings` is a reserved key
	// that is deleted afterwards, not a project. The settings pass below
	// covers those properly, so they are dropped here rather than reported
	// twice and wrongly.
	found := decodeUnused(k, "", projects)
	keys := make([]string, 0, len(found))
	for _, key := range found {
		if key == settingsKey || strings.HasPrefix(key, settingsKey+".") {
			continue
		}
		if key == deprecatedProjectsKey || strings.HasPrefix(key, deprecatedProjectsKey+".") {
			continue
		}
		keys = append(keys, key)
	}

	// Settings are unmarshalled from their own subtree, so the paths come back
	// relative to it; re-prefix them so a warning names the key as it is
	// written in the file.
	for _, key := range decodeUnused(k, settingsKey, settings) {
		keys = append(keys, settingsKey+"."+key)
	}

	sort.Strings(keys)
	return keys
}

// decodeUnused unmarshals the subtree at path into a throwaway copy of out's
// type and returns the keys the decoder did not consume. It decodes into the
// caller's value, which is already populated by the real unmarshal, so nothing
// here changes what the config resolved to.
func decodeUnused(k *koanf.Koanf, path string, out any) []string {
	var md mapstructure.Metadata
	conf := koanf.UnmarshalConf{
		Tag: configTag,
		DecoderConfig: &mapstructure.DecoderConfig{
			Metadata:         &md,
			Result:           out,
			WeaklyTypedInput: true,
			TagName:          configTag,
		},
	}
	if err := k.UnmarshalWithConf(path, out, conf); err != nil {
		// The real unmarshal already ran and reported any failure. This pass
		// exists only to collect key names, so a failure here means no
		// warnings rather than a second error about the same file.
		return nil
	}
	return normalizeKeys(md.Unused)
}

// normalizeKeys rewrites mapstructure's bracketed map syntax into the dotted
// path the user wrote in the config file: `[acme].repos[0].srcc` is reported
// as `acme.repos[0].srcc`. List indices are left alone — they say which entry
// of `repos:` is at fault, which is the useful half.
func normalizeKeys(unused []string) []string {
	keys := make([]string, 0, len(unused))
	for _, key := range unused {
		key = strings.ReplaceAll(key, "[", ".")
		key = strings.ReplaceAll(key, "]", "")
		key = strings.TrimPrefix(key, ".")
		// A map key holding a list index reads better with the bracket back:
		// `repos.0.srcc` is `repos[0].srcc` in the file.
		keys = append(keys, restoreIndices(key))
	}
	return keys
}

// restoreIndices turns a dotted numeric segment back into a bracketed index,
// so a path names the list entry the way the config file's reader would.
func restoreIndices(key string) string {
	parts := strings.Split(key, ".")
	var b strings.Builder
	for i, part := range parts {
		if i > 0 && isIndex(part) {
			fmt.Fprintf(&b, "[%s]", part)
			continue
		}
		if i > 0 {
			b.WriteByte('.')
		}
		b.WriteString(part)
	}
	return b.String()
}

// isIndex reports whether part is a list index rather than a key name.
func isIndex(part string) bool {
	if part == "" {
		return false
	}
	for _, r := range part {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
