package list

import (
	"strings"
	"testing"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/app/cli/clitest"
)

// labelProject returns a fixture project with its own labels.
func labelProject(t *testing.T, labels []domain.Label, repos ...clitest.Repo) domain.Project {
	t.Helper()
	project := clitest.NewProject(t, repos...)
	project.Labels = labels
	return project
}

// TestExecListProjectLabelReachesRepos checks that a project label selects all
// its repositories.
func TestExecListProjectLabelReachesRepos(t *testing.T) {
	t.Parallel()

	deps := clitest.New(t, clitest.FakeGit{})
	deps.Projects["acme"] = labelProject(t, []domain.Label{"work"},
		clitest.Cloned("api"), clitest.Cloned("web"))
	deps.Projects["other"] = clitest.NewProject(t, clitest.Cloned("stray"))

	err := ExecList("name", domain.NewLabelSet([]string{"work"}), nil, deps.RuntimeCLI)
	if err != nil {
		t.Fatalf("ExecList error = %v, want nil", err)
	}

	got := deps.Result()
	for _, want := range []string{"api", "web"} {
		if !strings.Contains(got, want) {
			t.Errorf("Result Output = %q, want the labeled project's %q", got, want)
		}
	}
	if strings.Contains(got, "stray") {
		t.Errorf("Result Output = %q, want the unlabeled project's repository absent", got)
	}
}

// TestExecListSubProjectInheritsLabel checks that Sub-projects inherit parent
// labels at any depth.
func TestExecListSubProjectInheritsLabel(t *testing.T) {
	t.Parallel()

	deps := clitest.New(t, clitest.FakeGit{})
	parent := labelProject(t, []domain.Label{"work"}, clitest.Cloned("api"))
	sub := clitest.NewProject(t, clitest.Cloned("db"))
	sub.Name = "infra"
	parent.SubProjects = []domain.Project{sub}
	deps.Projects["acme"] = parent

	err := ExecList("name", domain.NewLabelSet([]string{"work"}), nil, deps.RuntimeCLI)
	if err != nil {
		t.Fatalf("ExecList error = %v, want nil", err)
	}
	if got := deps.Result(); !strings.Contains(got, "db") {
		t.Errorf("Result Output = %q, want the sub-project's repository to inherit the label", got)
	}
}

// TestExecListUnmatchedLabelWarns checks that an unmatched label warns.
func TestExecListUnmatchedLabelWarns(t *testing.T) {
	t.Parallel()

	deps := clitest.New(t, clitest.FakeGit{}).
		WithProject("acme", clitest.Cloned("api").Labeled("demo"))

	err := ExecList("name", domain.NewLabelSet([]string{"ghost"}), nil, deps.RuntimeCLI)
	if err == nil {
		t.Fatal("ExecList with an unmatched label = nil, want a warning")
	}
	if !domain.IsWarning(err) {
		t.Errorf("ExecList error = %v, want a downgradeable warning", err)
	}
	if !strings.Contains(err.Error(), "ghost") {
		t.Errorf("warning = %q, want it to name the label", err)
	}
}

// TestExecListLabelJSONCarriesEffectiveLabels checks that JSON includes inherited
// labels.
func TestExecListLabelJSONCarriesEffectiveLabels(t *testing.T) {
	t.Parallel()

	deps := clitest.New(t, clitest.FakeGit{})
	deps.Projects["acme"] = labelProject(t, []domain.Label{"work"},
		clitest.Cloned("api").Labeled("demo"))

	if err := ExecList("json", domain.LabelSet{}, nil, deps.RuntimeCLI); err != nil {
		t.Fatalf("ExecList error = %v, want nil", err)
	}

	api, ok := deps.JSONRepos("acme")["api"]
	if !ok {
		t.Fatal("envelope holds no api repository")
	}
	labels, _ := api["labels"].([]any)
	if len(labels) != 2 || labels[0] != "demo" || labels[1] != "work" {
		t.Errorf("api labels = %v, want [demo work]: its own plus the project's", api["labels"])
	}
}
