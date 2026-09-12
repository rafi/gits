package main

import (
	"errors"
	"testing"

	"github.com/spf13/cobra"
)

// TestConfigLoadErrorGatesCommands proves a captured config parse/read failure
// aborts before any command runs: the root's PersistentPreRunE returns it, so
// the process exits non-zero instead of running against an empty config and
// misreporting the reason. A nil errConfigLoad lets commands run.
//
//nolint:paralleltest // mutates the package-level errConfigLoad; must stay serial.
func TestConfigLoadErrorGatesCommands(t *testing.T) {
	orig := errConfigLoad
	t.Cleanup(func() { errConfigLoad = orig })

	preRun := rootCmd.PersistentPreRunE
	if preRun == nil {
		t.Fatal("rootCmd has no PersistentPreRunE to gate on the config error")
	}

	sentinel := errors.New("unable to load config: broken")
	errConfigLoad = sentinel
	if err := preRun(&cobra.Command{}, nil); !errors.Is(err, sentinel) {
		t.Errorf("PersistentPreRunE with a config error = %v, want %v", err, sentinel)
	}

	errConfigLoad = nil
	if err := preRun(&cobra.Command{}, nil); err != nil {
		t.Errorf("PersistentPreRunE without a config error = %v, want nil", err)
	}
}
