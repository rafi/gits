package cli

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/rafi/gits/domain"
)

// tagFlagCommands are the commands that take `--tag`.
var tagFlagCommands = []string{
	"add", "checkout", "clone", "exec", "fetch",
	"list", "orphan", "pull", "push", "status",
}

// TestTagFlagRegistered checks that each command has its own `--tag/-t`
// destination.
//
//nolint:paralleltest // reads the shared command tree; kept serial with its siblings.
func TestTagFlagRegistered(t *testing.T) {
	registerRootFlags()

	for _, name := range tagFlagCommands {
		cmd, _, err := rootCmd.Find([]string{name})
		if err != nil || cmd.Name() != name {
			t.Errorf("%s: command not found in the tree", name)
			continue
		}
		flag := cmd.PersistentFlags().Lookup("tag")
		if flag == nil {
			t.Errorf("%s: no --tag flag", name)
			continue
		}
		if flag.Shorthand != "t" {
			t.Errorf("%s: --tag shorthand = %q, want t", name, flag.Shorthand)
		}
	}

	// Setting one command's --tag leaves the others empty.
	if err := pullCmd.PersistentFlags().Set("tag", "demo"); err != nil {
		t.Fatalf("set pull --tag: %v", err)
	}
	t.Cleanup(func() { _ = pullCmd.PersistentFlags().Set("tag", "") })

	if got := tagSet("pull"); got.String() != "demo" {
		t.Errorf("pull --tag = %q, want demo", got)
	}
	if got := tagSet("status"); !got.Empty() {
		t.Errorf("status --tag = %q, want it untouched by pull's", got)
	}
}

// TestTagFlagAcceptsBothForms checks that comma-separated and repeated `-t`
// yield the same set.
//
//nolint:paralleltest // mutates the shared command tree's flags; must stay serial.
func TestTagFlagAcceptsBothForms(t *testing.T) {
	registerRootFlags()

	flags := statusCmd.PersistentFlags()
	t.Cleanup(func() { _ = flags.Set("tag", "") })

	if err := flags.Set("tag", "demo,backend"); err != nil {
		t.Fatalf("set --tag=demo,backend: %v", err)
	}
	commaForm := tagSet("status")

	// String slice flags append, so reset first.
	if err := flags.Set("tag", ""); err != nil {
		t.Fatalf("reset --tag: %v", err)
	}
	*tagValues["status"] = nil
	for _, value := range []string{"backend", "demo"} {
		if err := flags.Set("tag", value); err != nil {
			t.Fatalf("set --tag=%s: %v", value, err)
		}
	}
	repeatedForm := tagSet("status")

	if commaForm != repeatedForm {
		t.Errorf("-t demo,backend = %q but repeated -t = %q, want the same set",
			commaForm, repeatedForm)
	}
	if commaForm.String() != "backend,demo" {
		t.Errorf("set = %q, want the tags normalized and sorted", commaForm)
	}
}

// TestPushAllTagsReplacesTags checks `--all-tags` works and `--tags` is a
// hidden, deprecated alias.
//
//nolint:paralleltest // mutates the shared pushCmd flags; must stay serial.
func TestPushAllTagsReplacesTags(t *testing.T) {
	registerRootFlags()

	flags := pushCmd.PersistentFlags()
	t.Cleanup(func() {
		pushOpts.AllTags = false
		_ = flags.Set("all-tags", "false")
		_ = flags.Set("tags", "false")
	})

	allTags := flags.Lookup("all-tags")
	if allTags == nil {
		t.Fatal("push has no --all-tags flag")
	}
	if err := flags.Set("all-tags", "true"); err != nil {
		t.Fatalf("set --all-tags: %v", err)
	}
	if !pushOpts.AllTags {
		t.Error("--all-tags did not reach the push options")
	}

	old := flags.Lookup("tags")
	if old == nil {
		t.Fatal("push lost --tags entirely, want the old spelling to keep working")
	}
	if old.Deprecated == "" {
		t.Error("--tags is not deprecated, want it to warn and point at --all-tags")
	}
	if !strings.Contains(old.Deprecated, "--all-tags") {
		t.Errorf("--tags deprecation = %q, want it to name --all-tags", old.Deprecated)
	}
	if !old.Hidden {
		t.Error("--tags is still offered in help, want a deprecated flag hidden")
	}

	// Both spellings set the same option.
	pushOpts.AllTags = false
	if err := flags.Set("tags", "true"); err != nil {
		t.Fatalf("set --tags: %v", err)
	}
	if !pushOpts.AllTags {
		t.Error("the deprecated --tags no longer reaches the push options")
	}
}

// TestPushTagAndAllTagsCoexist checks `--tag` and `--all-tags` are independent.
//
//nolint:paralleltest // mutates the shared pushCmd flags; must stay serial.
func TestPushTagAndAllTagsCoexist(t *testing.T) {
	registerRootFlags()

	flags := pushCmd.PersistentFlags()
	t.Cleanup(func() {
		pushOpts.AllTags = false
		_ = flags.Set("all-tags", "false")
		_ = flags.Set("tag", "")
		*tagValues["push"] = nil
	})

	if err := flags.Set("tag", "demo"); err != nil {
		t.Fatalf("set --tag: %v", err)
	}
	if err := flags.Set("all-tags", "true"); err != nil {
		t.Fatalf("set --all-tags: %v", err)
	}

	if got := tagSet("push"); got.String() != "demo" {
		t.Errorf("--tag = %q, want demo", got)
	}
	if !pushOpts.AllTags {
		t.Error("--all-tags was not recorded alongside --tag")
	}
}

// TestTagCompletionOffersConfiguredTags checks completion of project and
// repository tags, including the last element of a list.
//
//nolint:paralleltest // reads the package-level configFile; must stay serial.
func TestTagCompletionOffersConfiguredTags(t *testing.T) {
	previous := configFile.Projects
	t.Cleanup(func() { configFile.Projects = previous })

	configFile.Projects = domain.ProjectListKeyed{
		"acme": {
			Tags:  []domain.Tag{"work"},
			Repos: []domain.Repository{{Name: "api", Tags: []domain.Tag{"demo", "backend"}}},
		},
	}

	tests := []struct {
		name       string
		toComplete string
		want       []string
	}{
		{"everything", "", []string{"backend", "demo", "work"}},
		{"by prefix", "b", []string{"backend"}},
		{"a project tag", "w", []string{"work"}},
		{"nothing matches", "zz", nil},
		{"the last of a list", "demo,b", []string{"demo,backend"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, directive := completeTags(nil, nil, tt.toComplete)
			if directive != cobra.ShellCompDirectiveNoFileComp {
				t.Errorf("directive = %v, want no file completion", directive)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("completions = %q, want %q", got, tt.want)
			}
			for i, want := range tt.want {
				if got[i] != want {
					t.Errorf("completions = %q, want %q", got, tt.want)
					break
				}
			}
		})
	}
}
