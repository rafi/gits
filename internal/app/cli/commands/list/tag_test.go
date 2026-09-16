package list

import (
	"strings"
	"testing"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/app/cli/clitest"
)

// tagProject returns a fixture project with its own tags.
func tagProject(t *testing.T, tags []domain.Tag, repos ...clitest.Repo) domain.Project {
	t.Helper()
	project := clitest.NewProject(t, repos...)
	project.Tags = tags
	return project
}

// TestExecListProjectTagReachesRepos checks that a project tag selects all
// its repositories.
func TestExecListProjectTagReachesRepos(t *testing.T) {
	t.Parallel()

	deps := clitest.New(t, clitest.FakeGit{})
	deps.Projects["acme"] = tagProject(t, []domain.Tag{"work"},
		clitest.Cloned("api"), clitest.Cloned("web"))
	deps.Projects["other"] = clitest.NewProject(t, clitest.Cloned("stray"))

	err := ExecList("name", domain.NewTagSet([]string{"work"}), nil, deps.RuntimeCLI)
	if err != nil {
		t.Fatalf("ExecList error = %v, want nil", err)
	}

	got := deps.Result()
	for _, want := range []string{"api", "web"} {
		if !strings.Contains(got, want) {
			t.Errorf("Result Output = %q, want the tagged project's %q", got, want)
		}
	}
	if strings.Contains(got, "stray") {
		t.Errorf("Result Output = %q, want the untagged project's repository absent", got)
	}
}

// TestExecListSubProjectInheritsTag checks that Sub-projects inherit parent
// tags at any depth.
func TestExecListSubProjectInheritsTag(t *testing.T) {
	t.Parallel()

	deps := clitest.New(t, clitest.FakeGit{})
	parent := tagProject(t, []domain.Tag{"work"}, clitest.Cloned("api"))
	sub := clitest.NewProject(t, clitest.Cloned("db"))
	sub.Name = "infra"
	parent.SubProjects = []domain.Project{sub}
	deps.Projects["acme"] = parent

	err := ExecList("name", domain.NewTagSet([]string{"work"}), nil, deps.RuntimeCLI)
	if err != nil {
		t.Fatalf("ExecList error = %v, want nil", err)
	}
	if got := deps.Result(); !strings.Contains(got, "db") {
		t.Errorf("Result Output = %q, want the sub-project's repository to inherit the tag", got)
	}
}

// TestExecListUnmatchedTagWarns checks that an unmatched tag warns.
func TestExecListUnmatchedTagWarns(t *testing.T) {
	t.Parallel()

	deps := clitest.New(t, clitest.FakeGit{}).
		WithProject("acme", clitest.Cloned("api").Tagged("demo"))

	err := ExecList("name", domain.NewTagSet([]string{"ghost"}), nil, deps.RuntimeCLI)
	if err == nil {
		t.Fatal("ExecList with an unmatched tag = nil, want a warning")
	}
	if !domain.IsWarning(err) {
		t.Errorf("ExecList error = %v, want a downgradeable warning", err)
	}
	if !strings.Contains(err.Error(), "ghost") {
		t.Errorf("warning = %q, want it to name the tag", err)
	}
}

// TestExecListTagJSONCarriesEffectiveTags checks that JSON includes inherited
// tags.
func TestExecListTagJSONCarriesEffectiveTags(t *testing.T) {
	t.Parallel()

	deps := clitest.New(t, clitest.FakeGit{})
	deps.Projects["acme"] = tagProject(t, []domain.Tag{"work"},
		clitest.Cloned("api").Tagged("demo"))

	if err := ExecList("json", domain.TagSet{}, nil, deps.RuntimeCLI); err != nil {
		t.Fatalf("ExecList error = %v, want nil", err)
	}

	api, ok := deps.JSONRepos("acme")["api"]
	if !ok {
		t.Fatal("envelope holds no api repository")
	}
	tags, _ := api["tags"].([]any)
	if len(tags) != 2 || tags[0] != "demo" || tags[1] != "work" {
		t.Errorf("api tags = %v, want [demo work]: its own plus the project's", api["tags"])
	}
}
