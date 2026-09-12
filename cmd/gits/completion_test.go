package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"

	"github.com/spf13/cobra"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/bulk"
	"github.com/rafi/gits/internal/cli/config"
	"github.com/rafi/gits/internal/cli/list"
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

// TestFlagValueCompletion proves pressing Tab after `-o` or `-C` offers the
// values that flag accepts, filtered by what has been typed. The candidates
// come from the validators themselves — bulk.Formats, list.Formats,
// config.ColorChoices — and the drift test below pins that they stay the
// values the command accepts.
//
//nolint:paralleltest // reads the shared command tree; kept serial with its siblings.
func TestFlagValueCompletion(t *testing.T) {
	registerRootFlags()

	for _, tc := range []struct {
		name       string
		cmd        *cobra.Command
		flag       string
		toComplete string
		want       []string
	}{
		{"list -o offers every style", listCmd, "output", "", list.Formats()},
		{"list -o filters by prefix", listCmd, "output", "t", []string{"table", "tree"}},
		{"status -o is table and json", statusCmd, "output", "", bulk.Formats()},
		{"clone -o is table and json", cloneCmd, "output", "", bulk.Formats()},
		{"doctor -o is table and json", doctorCmd, "output", "", bulk.Formats()},
		{"exec -o is table and json", execCmd, "output", "", bulk.Formats()},
		{"fetch -o is table and json", fetchCmd, "output", "", bulk.Formats()},
		{"pull -o is table and json", pullCmd, "output", "", bulk.Formats()},
		{"push -o is table and json", pushCmd, "output", "", bulk.Formats()},
		{"-C offers the three colors", rootCmd, "color", "", config.ColorChoices()},
		{"-C filters by prefix", rootCmd, "color", "a", []string{"auto", "always"}},
		{"an unmatched prefix offers nothing", listCmd, "output", "z", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, ok := tc.cmd.GetFlagCompletionFunc(tc.flag)
			if !ok {
				t.Fatalf("%s --%s has no completion function registered", tc.cmd.Name(), tc.flag)
			}
			got, directive := f(tc.cmd, nil, tc.toComplete)
			if !slices.Equal(got, tc.want) {
				t.Errorf("completions = %v, want %v", got, tc.want)
			}
			if directive != cobra.ShellCompDirectiveNoFileComp {
				t.Errorf("directive = %v, want NoFileComp: a flag value is not a filename",
					directive)
			}
		})
	}
}

// TestFlagCompletionMatchesValidators proves every offered value is one the
// command would accept — the property ticket 25 asks for, and the one that
// breaks first when a format is added to a validator and not to its
// completion. It asserts against the validators rather than against a literal
// list, so a new format is covered the moment it is accepted.
//
//nolint:paralleltest // reads the shared command tree; kept serial with its siblings.
func TestFlagCompletionMatchesValidators(t *testing.T) {
	registerRootFlags()

	t.Run("every -o value passes its command's validator", func(t *testing.T) {
		for _, cmd := range []*cobra.Command{
			cloneCmd, doctorCmd, execCmd, fetchCmd, pullCmd, pushCmd, statusCmd,
		} {
			f, ok := cmd.GetFlagCompletionFunc("output")
			if !ok {
				t.Errorf("%s --output has no completion function", cmd.Name())
				continue
			}
			got, _ := f(cmd, nil, "")
			if len(got) == 0 {
				t.Errorf("%s --output offers nothing", cmd.Name())
			}
			for _, v := range got {
				if err := bulk.ValidateFormat(v); err != nil {
					t.Errorf("%s --output offers %q, which its validator rejects: %v",
						cmd.Name(), v, err)
				}
			}
		}
	})

	// `list`'s styles are pinned in its own package, where a fixture project
	// makes every one of them render: TestExecListWritesResultOutput drives
	// list.Formats() itself. Here it is enough that the flag offers that same
	// slice, which the case above asserts.

	t.Run("every -C value is accepted by the flag", func(t *testing.T) {
		f, ok := rootCmd.GetFlagCompletionFunc("color")
		if !ok {
			t.Fatal("--color has no completion function")
		}
		got, _ := f(rootCmd, nil, "")
		if len(got) == 0 {
			t.Fatal("--color offers nothing")
		}
		flag := rootCmd.PersistentFlags().Lookup("color")
		t.Cleanup(func() { _ = flag.Value.Set(config.ColorOptionDefault) })
		for _, v := range got {
			if err := flag.Value.Set(v); err != nil {
				t.Errorf("--color offers %q, which the flag rejects: %v", v, err)
			}
		}
	})
}

// TestCompleteProject covers the first positional argument: project names are
// offered with their descriptions, filtered by what has been typed.
//
//nolint:paralleltest // mutates package-level configFile; must stay serial.
func TestCompleteProject(t *testing.T) {
	orig := configFile
	t.Cleanup(func() { configFile = orig })

	configFile.Projects = domain.ProjectListKeyed{
		"acme":     {Desc: "all the things"},
		"acrobat":  {},
		"burrower": {Desc: "digs"},
	}

	t.Run("an empty prefix offers every project", func(t *testing.T) {
		got, directive := completeProject(nil, nil, "")
		if directive != cobra.ShellCompDirectiveNoFileComp {
			t.Errorf("directive = %v, want NoFileComp", directive)
		}
		slices.Sort(got)
		want := []string{"acme\tall the things", "acrobat\t", "burrower\tdigs"}
		if !slices.Equal(got, want) {
			t.Errorf("completions = %q, want %q", got, want)
		}
	})

	t.Run("a prefix filters", func(t *testing.T) {
		got, _ := completeProject(nil, nil, "ac")
		slices.Sort(got)
		want := []string{"acme\tall the things", "acrobat\t"}
		if !slices.Equal(got, want) {
			t.Errorf("completions = %q, want %q", got, want)
		}
	})

	t.Run("a prefix matching nothing offers nothing", func(t *testing.T) {
		if got, _ := completeProject(nil, nil, "zz"); got != nil {
			t.Errorf("completions = %q, want none", got)
		}
	})

	// With no argument yet, the repo and branch completers fall through to the
	// project one rather than offering nothing.
	t.Run("repo and branch completers fall through to projects", func(t *testing.T) {
		repo, _ := completeProjectRepo(nil, nil, "burr")
		branch, _ := completeProjectRepoBranch(nil, nil, "burr")
		want := []string{"burrower\tdigs"}
		if !slices.Equal(repo, want) {
			t.Errorf("completeProjectRepo(no args) = %q, want %q", repo, want)
		}
		if !slices.Equal(branch, want) {
			t.Errorf("completeProjectRepoBranch(no args) = %q, want %q", branch, want)
		}
	})
}

// TestCompleteProjectRepoOffersRepositories covers the second positional
// argument against a local project, which needs no provider and so completes
// from the configuration alone.
//
//nolint:paralleltest // mutates package-level configFile; must stay serial.
func TestCompleteProjectRepoOffersRepositories(t *testing.T) {
	orig := configFile
	t.Cleanup(func() { configFile = orig })

	configFile.Projects = domain.ProjectListKeyed{
		"acme": {
			Path: t.TempDir(),
			Repos: []domain.Repository{
				{Name: "api", Dir: "api"},
				{Name: "web", Dir: "web"},
			},
		},
	}

	t.Run("offers every repository", func(t *testing.T) {
		got, directive := completeProjectRepo(nil, []string{"acme"}, "")
		if directive != cobra.ShellCompDirectiveNoFileComp {
			t.Errorf("directive = %v, want NoFileComp", directive)
		}
		slices.Sort(got)
		if want := []string{"api", "web"}; !slices.Equal(got, want) {
			t.Errorf("completions = %q, want %q", got, want)
		}
	})

	t.Run("a prefix filters", func(t *testing.T) {
		got, _ := completeProjectRepo(nil, []string{"acme"}, "a")
		if want := []string{"api"}; !slices.Equal(got, want) {
			t.Errorf("completions = %q, want %q", got, want)
		}
	})

	// An unknown project name is not an error at the prompt: Tab stays silent.
	t.Run("an unknown project offers nothing", func(t *testing.T) {
		got, directive := completeProjectRepo(nil, []string{"nope"}, "")
		if got != nil {
			t.Errorf("completions = %q, want none", got)
		}
		if directive != cobra.ShellCompDirectiveNoFileComp {
			t.Errorf("directive = %v, want NoFileComp, never Error", directive)
		}
	})

	// The branch completer needs a repository it can find; an unknown one
	// stops rather than reaching git with an empty path.
	t.Run("an unknown repository offers no branches", func(t *testing.T) {
		got, directive := completeProjectRepoBranch(nil, []string{"acme", "nope"}, "")
		if got != nil {
			t.Errorf("completions = %q, want none", got)
		}
		if directive != cobra.ShellCompDirectiveNoFileComp {
			t.Errorf("directive = %v, want NoFileComp", directive)
		}
	})
}

// TestCompleteProjectRepoBranchOffersBranches drives the branch completer
// against a real repository on disk, which is what deps.Git.Branches reads.
//
//nolint:paralleltest // mutates package-level configFile; must stay serial.
func TestCompleteProjectRepoBranchOffersBranches(t *testing.T) {
	orig := configFile
	t.Cleanup(func() { configFile = orig })

	root := t.TempDir()
	repoPath := filepath.Join(root, "api")
	gitInit(t, repoPath)

	configFile.Projects = domain.ProjectListKeyed{
		"acme": {Path: root, Repos: []domain.Repository{{Name: "api", Dir: "api"}}},
	}

	got, directive := completeProjectRepoBranch(nil, []string{"acme", "api"}, "")
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("directive = %v, want NoFileComp", directive)
	}
	if !slices.Contains(got, "main") {
		t.Errorf("completions = %q, want them to contain %q", got, "main")
	}

	// A prefix that matches nothing offers nothing, rather than everything.
	if got, _ := completeProjectRepoBranch(nil, []string{"acme", "api"}, "zz"); got != nil {
		t.Errorf("completions = %q, want none for a non-matching prefix", got)
	}
}

// gitInit creates a repository with one commit on `main`.
func gitInit(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	for _, args := range [][]string{
		{"init", "-q", "--initial-branch=main"},
		{"-c", "user.email=t@example.com", "-c", "user.name=t", "commit", "-q", "--allow-empty", "-m", "init"},
	} {
		cmd := exec.Command("git", append([]string{"-C", path}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
}
