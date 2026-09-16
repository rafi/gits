package pull

import (
	"slices"
	"strings"
	"testing"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/app/cli/clitest"
)

// TestExecPullLabelSelectsRepos checks that only labeled repositories are
// pulled.
func TestExecPullLabelSelectsRepos(t *testing.T) {
	t.Parallel()

	g := tracking()
	deps := clitest.New(t, g).WithProject("acme",
		clitest.Cloned("api").Labeled("demo"),
		clitest.Cloned("web"),
		clitest.Cloned("docs").Labeled("demo"),
	)

	err := ExecPull("table", domain.NewLabelSet([]string{"demo"}), []string{"acme"}, deps.RuntimeCLI)
	if err != nil {
		t.Fatalf("ExecPull error = %v, want nil", err)
	}

	got := g.Pulled()
	if len(got) != 2 || !slices.Contains(got, "api") || !slices.Contains(got, "docs") {
		t.Errorf("pulled = %q, want exactly the labeled api and docs", got)
	}
	if result := deps.Result(); strings.Contains(result, "web") {
		t.Errorf("Result Output = %q, want the unlabeled repository absent", result)
	}
}

// TestExecPullSeveralLabelsAreAUnion checks that several labels match any.
func TestExecPullSeveralLabelsAreAUnion(t *testing.T) {
	t.Parallel()

	g := tracking()
	deps := clitest.New(t, g).WithProject("acme",
		clitest.Cloned("api").Labeled("demo"),
		clitest.Cloned("web").Labeled("backend"),
		clitest.Cloned("docs"),
	)

	err := ExecPull("table", domain.NewLabelSet([]string{"demo,backend"}),
		[]string{"acme"}, deps.RuntimeCLI)
	if err != nil {
		t.Fatalf("ExecPull error = %v, want nil", err)
	}

	got := g.Pulled()
	if len(got) != 2 || !slices.Contains(got, "api") || !slices.Contains(got, "web") {
		t.Errorf("pulled = %q, want both api (demo) and web (backend)", got)
	}
}

// TestExecPullUnmatchedLabelWarns checks that an unmatched label is a warning.
func TestExecPullUnmatchedLabelWarns(t *testing.T) {
	t.Parallel()

	g := tracking()
	deps := clitest.New(t, g).WithProject("acme",
		clitest.Cloned("api").Labeled("demo"))

	err := ExecPull("table", domain.NewLabelSet([]string{"ghost"}), []string{"acme"}, deps.RuntimeCLI)
	if err == nil {
		t.Fatal("ExecPull with an unmatched label = nil, want a warning")
	}
	if !domain.IsWarning(err) {
		t.Errorf("ExecPull error = %v, want a downgradeable warning", err)
	}
	if !strings.Contains(err.Error(), "ghost") {
		t.Errorf("warning = %q, want it to name the label asked for", err)
	}
	if got := g.Pulled(); len(got) != 0 {
		t.Errorf("pulled = %q, want nothing pulled", got)
	}
}

// TestExecPullNoLabelActsOnEverything checks that all repositories are pulled
// without `--label`.
func TestExecPullNoLabelActsOnEverything(t *testing.T) {
	t.Parallel()

	g := tracking()
	deps := clitest.New(t, g).WithProject("acme",
		clitest.Cloned("api").Labeled("demo"),
		clitest.Cloned("web"),
	)

	if err := ExecPull("table", domain.LabelSet{}, []string{"acme"}, deps.RuntimeCLI); err != nil {
		t.Fatalf("ExecPull error = %v, want nil", err)
	}
	if got := g.Pulled(); len(got) != 2 {
		t.Errorf("pulled = %q, want every repository", got)
	}
}

// TestExecPullLabelWithNamedRepo checks that naming an unlabeled repository
// reports it lacks the label.
func TestExecPullLabelWithNamedRepo(t *testing.T) {
	t.Parallel()

	// Parallel subtests need their own fakes.
	newDeps := func(t *testing.T) *clitest.Deps {
		t.Helper()
		return clitest.New(t, tracking()).WithProject("acme",
			clitest.Cloned("api").Labeled("demo"),
			clitest.Cloned("web"),
		)
	}

	t.Run("a labeled repository runs", func(t *testing.T) {
		t.Parallel()

		deps := newDeps(t)
		err := ExecPull("table", domain.NewLabelSet([]string{"demo"}),
			[]string{"acme", "api"}, deps.RuntimeCLI)
		if err != nil {
			t.Fatalf("ExecPull error = %v, want nil", err)
		}
	})

	t.Run("an unlabeled repository is reported as unlabeled", func(t *testing.T) {
		t.Parallel()

		deps := newDeps(t)
		err := ExecPull("table", domain.NewLabelSet([]string{"demo"}),
			[]string{"acme", "web"}, deps.RuntimeCLI)
		if err == nil {
			t.Fatal("ExecPull = nil, want the unlabeled repository refused")
		}
		if !strings.Contains(err.Error(), "does not carry label") {
			t.Errorf("error = %q, want it to say the repository is not labeled", err)
		}
		if !strings.Contains(err.Error(), "web") || !strings.Contains(err.Error(), "demo") {
			t.Errorf("error = %q, want it to name both the repository and the label", err)
		}
	})
}

// TestExecPullLabelJSONCarriesLabels checks that JSON includes effective labels.
func TestExecPullLabelJSONCarriesLabels(t *testing.T) {
	t.Parallel()

	g := tracking()
	deps := clitest.New(t, g).WithProject("acme",
		clitest.Cloned("api").Labeled("demo"))

	err := ExecPull("json", domain.NewLabelSet([]string{"demo"}), []string{"acme"}, deps.RuntimeCLI)
	if err != nil {
		t.Fatalf("ExecPull error = %v, want nil", err)
	}

	repos := deps.JSONRepos("acme")
	api, ok := repos["api"]
	if !ok {
		t.Fatalf("envelope = %v, want the api repository", repos)
	}
	labels, ok := api["labels"].([]any)
	if !ok || len(labels) != 1 || labels[0] != "demo" {
		t.Errorf("api labels = %v, want [demo]", api["labels"])
	}
}
