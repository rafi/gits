package cli

import (
	"errors"
	"os"
	"strings"
	"testing"
)

// TestPreviewCommandQuotesPathsWithSpaces: an fzf preview runs through a
// shell, so the binary's own path and the config path must survive spaces and
// single quotes as one word each — while fzf's own placeholders stay
// unquoted so fzf can expand them.
func TestPreviewCommandQuotesPathsWithSpaces(t *testing.T) {
	t.Parallel()

	got := previewCommandf(
		"/Users/a b/Library/Application Support/gits/config.yaml",
		"branch-overview", "{2}", "my project", "acme/api")

	want := "--config='/Users/a b/Library/Application Support/gits/config.yaml'"
	if !strings.Contains(got, want) {
		t.Errorf("preview = %q, want it to contain %q", got, want)
	}
	if !strings.HasSuffix(got, " branch-overview 'my project' 'acme/api' {2}") {
		t.Errorf("preview = %q, want quoted arguments and a bare {2}", got)
	}

	exe, err := os.Executable()
	if err != nil {
		t.Skipf("os.Executable: %v", err)
	}
	if !strings.HasPrefix(got, shellQuote(exe)+" ") {
		t.Errorf("preview = %q, want it to begin with this binary's quoted path %q", got, exe)
	}
}

// TestShellQuoteEmbeddedQuote: a single quote is the one character single
// quoting cannot carry, so it is spliced in instead.
func TestShellQuoteEmbeddedQuote(t *testing.T) {
	t.Parallel()

	if got, want := shellQuote("it's"), `'it'\''s'`; got != want {
		t.Errorf("shellQuote(\"it's\") = %s, want %s", got, want)
	}
}

// TestResolveSelfFallsBackToBareName: when [os.Executable] cannot answer, the
// bare name is all that is left to try.
func TestResolveSelfFallsBackToBareName(t *testing.T) {
	t.Parallel()

	fails := func() (string, error) { return "", errors.New("no /proc") }
	empty := func() (string, error) { return "", nil }
	found := func() (string, error) { return "/opt/bin/gits-dev", nil }

	if got := resolveSelf(fails); got != "gits" {
		t.Errorf("resolveSelf(failing) = %q, want gits", got)
	}
	if got := resolveSelf(empty); got != "gits" {
		t.Errorf("resolveSelf(empty) = %q, want gits", got)
	}
	if got := resolveSelf(found); got != "/opt/bin/gits-dev" {
		t.Errorf("resolveSelf = %q, want the resolved path", got)
	}
}
