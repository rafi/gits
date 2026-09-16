package cli

import (
	"testing"

	"github.com/spf13/cobra"

	"github.com/rafi/gits/domain"
)

// labelFlagCommands are the commands that take `--label`.
var labelFlagCommands = []string{
	"add", "checkout", "clone", "exec", "fetch",
	"list", "orphan", "pull", "push", "status",
}

// TestLabelFlagRegistered checks that each command has its own `--label/-l`
// destination.
//
//nolint:paralleltest // reads the shared command tree; kept serial with its siblings.
func TestLabelFlagRegistered(t *testing.T) {
	registerRootFlags()

	for _, name := range labelFlagCommands {
		cmd, _, err := rootCmd.Find([]string{name})
		if err != nil || cmd.Name() != name {
			t.Errorf("%s: command not found in the tree", name)
			continue
		}
		flag := cmd.PersistentFlags().Lookup("label")
		if flag == nil {
			t.Errorf("%s: no --label flag", name)
			continue
		}
		if flag.Shorthand != "l" {
			t.Errorf("%s: --label shorthand = %q, want l", name, flag.Shorthand)
		}
	}

	// Setting one command's --label leaves the others empty.
	if err := pullCmd.PersistentFlags().Set("label", "demo"); err != nil {
		t.Fatalf("set pull --label: %v", err)
	}
	t.Cleanup(func() { _ = pullCmd.PersistentFlags().Set("label", "") })

	if got := labelSet("pull"); got.String() != "demo" {
		t.Errorf("pull --label = %q, want demo", got)
	}
	if got := labelSet("status"); !got.Empty() {
		t.Errorf("status --label = %q, want it untouched by pull's", got)
	}
}

// TestLabelFlagAcceptsBothForms checks that comma-separated and repeated `-l`
// yield the same set.
//
//nolint:paralleltest // mutates the shared command tree's flags; must stay serial.
func TestLabelFlagAcceptsBothForms(t *testing.T) {
	registerRootFlags()

	flags := statusCmd.PersistentFlags()
	t.Cleanup(func() { _ = flags.Set("label", "") })

	if err := flags.Set("label", "demo,backend"); err != nil {
		t.Fatalf("set --label=demo,backend: %v", err)
	}
	commaForm := labelSet("status")

	// String slice flags append, so reset first.
	if err := flags.Set("label", ""); err != nil {
		t.Fatalf("reset --label: %v", err)
	}
	*labelValues["status"] = nil
	for _, value := range []string{"backend", "demo"} {
		if err := flags.Set("label", value); err != nil {
			t.Fatalf("set --label=%s: %v", value, err)
		}
	}
	repeatedForm := labelSet("status")

	if commaForm != repeatedForm {
		t.Errorf("-l demo,backend = %q but repeated -l = %q, want the same set",
			commaForm, repeatedForm)
	}
	if commaForm.String() != "backend,demo" {
		t.Errorf("set = %q, want the labels normalized and sorted", commaForm)
	}
}

// TestPushLabelAndTagsCoexist checks `--label` and `--tags` are independent.
//
//nolint:paralleltest // mutates the shared pushCmd flags; must stay serial.
func TestPushLabelAndTagsCoexist(t *testing.T) {
	registerRootFlags()

	flags := pushCmd.PersistentFlags()
	t.Cleanup(func() {
		pushOpts.Tags = false
		_ = flags.Set("tags", "false")
		_ = flags.Set("label", "")
		*labelValues["push"] = nil
	})

	if err := flags.Set("label", "demo"); err != nil {
		t.Fatalf("set --label: %v", err)
	}
	if err := flags.Set("tags", "true"); err != nil {
		t.Fatalf("set --tags: %v", err)
	}

	if got := labelSet("push"); got.String() != "demo" {
		t.Errorf("--label = %q, want demo", got)
	}
	if !pushOpts.Tags {
		t.Error("--tags was not recorded alongside --label")
	}
}

// TestLabelCompletionOffersConfiguredLabels checks completion of project and
// repository labels, including the last element of a list.
//
//nolint:paralleltest // reads the package-level configFile; must stay serial.
func TestLabelCompletionOffersConfiguredLabels(t *testing.T) {
	previous := configFile.Projects
	t.Cleanup(func() { configFile.Projects = previous })

	configFile.Projects = domain.ProjectListKeyed{
		"acme": {
			Labels: []domain.Label{"work"},
			Repos:  []domain.Repository{{Name: "api", Labels: []domain.Label{"demo", "backend"}}},
		},
	}

	tests := []struct {
		name       string
		toComplete string
		want       []string
	}{
		{"everything", "", []string{"backend", "demo", "work"}},
		{"by prefix", "b", []string{"backend"}},
		{"a project label", "w", []string{"work"}},
		{"nothing matches", "zz", nil},
		{"the last of a list", "demo,b", []string{"demo,backend"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, directive := completeLabels(nil, nil, tt.toComplete)
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
