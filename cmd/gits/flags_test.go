package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"

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

// TestBulkOutputFlagRegistered proves each line Bulk Command takes -o, bound
// to its own destination and defaulting to table, so `gits pull -o json`
// parses on every one of them and none share a variable by accident.
//
//nolint:paralleltest // reads the shared command tree; kept serial with its siblings.
func TestBulkOutputFlagRegistered(t *testing.T) {
	registerRootFlags()

	for _, tc := range []struct {
		cmd  *cobra.Command
		dest *string
	}{
		{cloneCmd, &cloneOutput},
		{execCmd, &execOutput},
		{fetchCmd, &fetchOutput},
		{pullCmd, &pullOutput},
		{pushCmd, &pushOutput},
	} {
		flag := tc.cmd.PersistentFlags().Lookup("output")
		if flag == nil {
			t.Errorf("%s: no --output flag", tc.cmd.Name())
			continue
		}
		if flag.Shorthand != "o" || flag.DefValue != "table" {
			t.Errorf("%s: --output shorthand/default = %q/%q, want o/table",
				tc.cmd.Name(), flag.Shorthand, flag.DefValue)
		}
		if err := flag.Value.Set("json"); err != nil {
			t.Errorf("%s: set --output=json: %v", tc.cmd.Name(), err)
		}
		if *tc.dest != "json" {
			t.Errorf("%s: --output=json left its destination at %q", tc.cmd.Name(), *tc.dest)
		}
		_ = flag.Value.Set("table")
	}
}
