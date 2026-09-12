package main

import (
	"testing"

	"github.com/spf13/cobra"
)

// TestCompletionArgClamps proves completion stops suggesting once a command's
// maximum argument count is reached, instead of offering values cobra will
// reject.
func TestCompletionArgClamps(t *testing.T) {
	t.Run("project+repo commands stop after 2 args", func(t *testing.T) {
		got, directive := completeProjectRepo(nil, []string{"proj", "repo"}, "")
		if got != nil || directive != cobra.ShellCompDirectiveNoFileComp {
			t.Errorf("completeProjectRepo(2 args) = (%v, %v), want (nil, NoFileComp)",
				got, directive)
		}
	})

	t.Run("project+repo+branch commands stop after 3 args", func(t *testing.T) {
		got, directive := completeProjectRepoBranch(nil, []string{"proj", "repo", "branch"}, "")
		if got != nil || directive != cobra.ShellCompDirectiveNoFileComp {
			t.Errorf("completeProjectRepoBranch(3 args) = (%v, %v), want (nil, NoFileComp)",
				got, directive)
		}
	})
}

// TestCompletionDepsSettings proves completion runs with the loaded settings
// (cache toggle, includeArchived, provider timeout) instead of zero values.
func TestCompletionDepsSettings(t *testing.T) {
	orig := configFile.Settings
	configFile.Settings.IncludeArchived = true
	configFile.Settings.ProviderTimeout = "42s"
	t.Cleanup(func() { configFile.Settings = orig })

	deps, err := completionDeps()
	if err != nil {
		t.Fatalf("completionDeps: %v", err)
	}
	if !deps.Settings.IncludeArchived || deps.Settings.ProviderTimeout != "42s" {
		t.Errorf("completionDeps Settings = %+v, want loaded settings", deps.Settings)
	}
}
