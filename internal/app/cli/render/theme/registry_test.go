package theme

import (
	"regexp"
	"slices"
	"strings"
	"testing"
)

func TestNames(t *testing.T) {
	t.Parallel()
	names := Names()
	if len(names) != 31 {
		t.Errorf("Names() has %d themes, want 31", len(names))
	}
	if !slices.IsSorted(names) {
		t.Error("Names() is not sorted")
	}
	kebab := regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	for _, n := range names {
		if !kebab.MatchString(n) {
			t.Errorf("theme name %q is not kebab-case", n)
		}
		th, err := Lookup(n)
		if err != nil || th.Name != n {
			t.Errorf("Lookup(%q) = %q, %v", n, th.Name, err)
		}
	}
}

func TestLookupUnknownListsNames(t *testing.T) {
	t.Parallel()
	_, err := Lookup("dracla")
	if err == nil || !strings.Contains(err.Error(), "dracula") {
		t.Errorf("Lookup(dracla) error = %v, want one listing the valid names", err)
	}
}

func TestThemeValueRejectsUnknown(t *testing.T) {
	t.Parallel()
	var dst string
	v := NewThemeValue(&dst)
	if err := v.Set("nord"); err != nil || dst != "nord" {
		t.Errorf("Set(nord) = %v, dst %q", err, dst)
	}
	if err := v.Set("nope"); err == nil {
		t.Error("Set(nope) error = nil, want rejection")
	}
}
