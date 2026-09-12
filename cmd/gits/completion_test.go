package main

import (
	"testing"

	"github.com/spf13/cobra"

	"github.com/rafi/gits/domain"
)

// TestCompletionNeverReachesProvider proves pressing Tab on a remote,
// provider-backed project with a cold cache offers no candidates and returns
// NoFileComp rather than an error — and, crucially, never runs the project's
// tokenCommand or a network fetch. The token command is set to one that fails
// loudly (`exit 1`); a fall-through to the provider would surface that failure,
// so a clean, empty completion is what proves the provider was not reached.
//
//nolint:paralleltest // mutates package-level configFile; must stay serial.
func TestCompletionNeverReachesProvider(t *testing.T) {
	orig := configFile
	t.Cleanup(func() { configFile = orig })

	configFile.Projects = domain.ProjectListKeyed{
		"remote": {
			Source: &domain.ProviderSource{Type: "github", Search: "acme"},
		},
	}
	// A token command that would fail loudly if the provider path were reached.
	configFile.Settings = domain.Settings{
		GitHub: domain.ProviderSettings{TokenCmd: "exit 1"},
	}

	got, directive := completeProjectRepo(nil, []string{"remote"}, "")
	if got != nil {
		t.Errorf("completions = %v, want none for a cold cache-only remote project", got)
	}
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("directive = %v, want NoFileComp (never Error, never a prompt)", directive)
	}
}

// TestCompletionArgClamps proves completion stops suggesting once a command's
// maximum argument count is reached, instead of offering values cobra will
// reject.
func TestCompletionArgClamps(t *testing.T) {
	t.Parallel()

	t.Run("project+repo commands stop after 2 args", func(t *testing.T) {
		t.Parallel()

		got, directive := completeProjectRepo(nil, []string{"proj", "repo"}, "")
		if got != nil || directive != cobra.ShellCompDirectiveNoFileComp {
			t.Errorf("completeProjectRepo(2 args) = (%v, %v), want (nil, NoFileComp)",
				got, directive)
		}
	})

	t.Run("project+repo+branch commands stop after 3 args", func(t *testing.T) {
		t.Parallel()

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
	t.Parallel()

	orig := configFile.Settings
	configFile.Settings.IncludeArchived = true
	configFile.Settings.ProviderTimeout = "42s"
	t.Cleanup(func() { configFile.Settings = orig })

	deps := completionDeps()
	if !deps.Settings.IncludeArchived || deps.Settings.ProviderTimeout != "42s" {
		t.Errorf("completionDeps Settings = %+v, want loaded settings", deps.Settings)
	}
}
