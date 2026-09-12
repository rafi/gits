package config

import (
	"reflect"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/rafi/gits/domain"
)

// styleFields returns the names of every field of structType whose type is
// styleType, in declaration order.
func styleFields(structType, styleType reflect.Type) []string {
	var names []string
	for i := range structType.NumField() {
		if structType.Field(i).Type == styleType {
			names = append(names, structType.Field(i).Name)
		}
	}
	return names
}

// TestThemeParity pins both sides of the bridge ParseConfig reflects over: it
// looks a domain.Theme style up on the CLI Theme by field name, so a style
// declared on one side alone panics while loading a user's config. Drift
// fails here instead, naming the field.
//
// Only the styles are paired. A CLI field of another type — the list table's
// border — is not configurable and is not claimed to be.
func TestThemeParity(t *testing.T) {
	t.Parallel()

	var (
		domainTheme = reflect.TypeFor[domain.Theme]()
		domainStyle = reflect.TypeFor[domain.Style]()
		cliTheme    = reflect.TypeFor[Theme]()
		cliStyle    = reflect.TypeFor[lipgloss.Style]()
	)

	// ParseConfig selects a domain field by its type's *name* and then asserts
	// it to domain.Style, so that is the predicate mirrored here: a field the
	// bridge picks up but cannot assert panics just as loudly as one with no
	// counterpart.
	var domainNames []string
	for i := range domainTheme.NumField() {
		field := domainTheme.Field(i)
		switch {
		case field.Type.Name() != domainStyle.Name():
		case field.Type != domainStyle:
			t.Errorf("domain.Theme.%s is %s, not domain.Style: ParseConfig "+
				"picks the field up by type name and panics asserting it",
				field.Name, field.Type)
		default:
			domainNames = append(domainNames, field.Name)
		}
	}
	cliNames := styleFields(cliTheme, cliStyle)
	// Both sides declare styles as named fields; a shape that stops holding
	// them that way would leave the assertions below with nothing to compare
	// and pass vacuously.
	if len(domainNames) == 0 || len(cliNames) == 0 {
		t.Fatalf("no styles found: domain.Theme has %d, config.Theme has %d",
			len(domainNames), len(cliNames))
	}

	configurable := make(map[string]bool, len(domainNames))
	for _, name := range domainNames {
		configurable[name] = true
		field, ok := cliTheme.FieldByName(name)
		switch {
		case !ok:
			t.Errorf("domain.Theme.%s has no config.Theme.%s: add the "+
				"lipgloss.Style field, or ParseConfig panics on a config "+
				"that sets the style", name, name)
		case field.Type != cliStyle:
			t.Errorf("config.Theme.%s is %s, not lipgloss.Style: "+
				"ParseConfig panics assigning domain.Theme.%s to it",
				name, field.Type, name)
		}
	}

	for _, name := range cliNames {
		if !configurable[name] {
			t.Errorf("config.Theme.%s has no domain.Theme.%s of type Style: "+
				"add the domain field, or the style can never be configured",
				name, name)
		}
	}
}
