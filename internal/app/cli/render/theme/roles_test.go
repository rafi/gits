package theme

import (
	"reflect"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/rafi/gits/internal/app/cli/render/style"
)

// TestRolesCoverTheme: every lipgloss.Style field of style.Theme is reachable
// from exactly one Role, and every Role has a mapping entry, so a style added
// to style.Theme without a role fails here rather than rendering unstyled.
func TestRolesCoverTheme(t *testing.T) {
	t.Parallel()
	styleType := reflect.TypeFor[lipgloss.Style]()
	want := map[string]bool{}
	for f := range reflect.TypeFor[style.Theme]().Fields() {
		if f.Type == styleType {
			want[f.Name] = true
		}
	}
	var th style.Theme
	seen := map[*lipgloss.Style]Role{}
	for _, r := range Roles() {
		field, ok := roleFields[r]
		if !ok {
			t.Fatalf("role %q has no style.Theme field", r)
		}
		ptr := field(&th)
		if prev, dup := seen[ptr]; dup {
			t.Errorf("roles %q and %q share a field", prev, r)
		}
		seen[ptr] = r
		if _, ok := baseMapping[r]; !ok {
			t.Errorf("role %q has no entry in baseMapping", r)
		}
	}
	if len(seen) != len(want) {
		t.Errorf("roles cover %d style fields, style.Theme has %d", len(seen), len(want))
	}
}
