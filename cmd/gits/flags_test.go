package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/rafi/gits/internal/version"
)

// TestRootVersionFlag proves --version prints the same string as `gits
// version`: the rootCmd.Version carries the semver and Go version, and the
// template prefixes "gits ".
//
//nolint:paralleltest // mutates the shared rootCmd (args, output); must stay serial.
func TestRootVersionFlag(t *testing.T) {
	registerRootFlags()

	want := version.GetVersion()
	if !strings.Contains(rootCmd.Version, want) {
		t.Errorf("rootCmd.Version = %q, want it to contain %q", rootCmd.Version, want)
	}

	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs([]string{"--version"})
	t.Cleanup(func() { rootCmd.SetArgs(nil) })

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("gits --version = %v, want nil", err)
	}
	got := out.String()
	if !strings.HasPrefix(got, "gits ") || !strings.Contains(got, want) {
		t.Errorf("--version output = %q, want it to start with 'gits ' and contain %q", got, want)
	}
}

// TestColorFlagRejectsUnknown proves the -C/--color flag rejects a value that
// is not auto|always|never, rather than silently meaning auto.
//
//nolint:paralleltest // mutates the shared rootCmd's color flag; must stay serial.
func TestColorFlagRejectsUnknown(t *testing.T) {
	registerRootFlags()

	if err := rootCmd.PersistentFlags().Set("color", "sometimes"); err == nil {
		t.Fatal("setting --color=sometimes = nil, want a rejected value")
	}
	// Restore a valid value so the shared rootCmd is left usable.
	t.Cleanup(func() { _ = rootCmd.PersistentFlags().Set("color", "auto") })

	if err := rootCmd.PersistentFlags().Set("color", "always"); err != nil {
		t.Errorf("setting --color=always = %v, want nil", err)
	}
}
