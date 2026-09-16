package pull

import (
	"slices"
	"strings"
	"testing"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/app/cli/clitest"
)

// TestExecPullTagSelectsRepos checks that only tagged repositories are
// pulled.
func TestExecPullTagSelectsRepos(t *testing.T) {
	t.Parallel()

	g := tracking()
	deps := clitest.New(t, g).WithProject("acme",
		clitest.Cloned("api").Tagged("demo"),
		clitest.Cloned("web"),
		clitest.Cloned("docs").Tagged("demo"),
	)

	err := ExecPull("table", domain.NewTagSet([]string{"demo"}), []string{"acme"}, deps.RuntimeCLI)
	if err != nil {
		t.Fatalf("ExecPull error = %v, want nil", err)
	}

	got := g.Pulled()
	if len(got) != 2 || !slices.Contains(got, "api") || !slices.Contains(got, "docs") {
		t.Errorf("pulled = %q, want exactly the tagged api and docs", got)
	}
	if result := deps.Result(); strings.Contains(result, "web") {
		t.Errorf("Result Output = %q, want the untagged repository absent", result)
	}
}

// TestExecPullSeveralTagsAreAUnion checks that several tags match any.
func TestExecPullSeveralTagsAreAUnion(t *testing.T) {
	t.Parallel()

	g := tracking()
	deps := clitest.New(t, g).WithProject("acme",
		clitest.Cloned("api").Tagged("demo"),
		clitest.Cloned("web").Tagged("backend"),
		clitest.Cloned("docs"),
	)

	err := ExecPull("table", domain.NewTagSet([]string{"demo,backend"}),
		[]string{"acme"}, deps.RuntimeCLI)
	if err != nil {
		t.Fatalf("ExecPull error = %v, want nil", err)
	}

	got := g.Pulled()
	if len(got) != 2 || !slices.Contains(got, "api") || !slices.Contains(got, "web") {
		t.Errorf("pulled = %q, want both api (demo) and web (backend)", got)
	}
}

// TestExecPullUnmatchedTagWarns checks that an unmatched tag is a warning.
func TestExecPullUnmatchedTagWarns(t *testing.T) {
	t.Parallel()

	g := tracking()
	deps := clitest.New(t, g).WithProject("acme",
		clitest.Cloned("api").Tagged("demo"))

	err := ExecPull("table", domain.NewTagSet([]string{"ghost"}), []string{"acme"}, deps.RuntimeCLI)
	if err == nil {
		t.Fatal("ExecPull with an unmatched tag = nil, want a warning")
	}
	if !domain.IsWarning(err) {
		t.Errorf("ExecPull error = %v, want a downgradeable warning", err)
	}
	if !strings.Contains(err.Error(), "ghost") {
		t.Errorf("warning = %q, want it to name the tag asked for", err)
	}
	if got := g.Pulled(); len(got) != 0 {
		t.Errorf("pulled = %q, want nothing pulled", got)
	}
}

// TestExecPullNoTagActsOnEverything checks that all repositories are pulled
// without `--tag`.
func TestExecPullNoTagActsOnEverything(t *testing.T) {
	t.Parallel()

	g := tracking()
	deps := clitest.New(t, g).WithProject("acme",
		clitest.Cloned("api").Tagged("demo"),
		clitest.Cloned("web"),
	)

	if err := ExecPull("table", domain.TagSet{}, []string{"acme"}, deps.RuntimeCLI); err != nil {
		t.Fatalf("ExecPull error = %v, want nil", err)
	}
	if got := g.Pulled(); len(got) != 2 {
		t.Errorf("pulled = %q, want every repository", got)
	}
}

// TestExecPullTagWithNamedRepo checks that naming an untagged repository
// reports it lacks the tag.
func TestExecPullTagWithNamedRepo(t *testing.T) {
	t.Parallel()

	// Parallel subtests need their own fakes.
	newDeps := func(t *testing.T) *clitest.Deps {
		t.Helper()
		return clitest.New(t, tracking()).WithProject("acme",
			clitest.Cloned("api").Tagged("demo"),
			clitest.Cloned("web"),
		)
	}

	t.Run("a tagged repository runs", func(t *testing.T) {
		t.Parallel()

		deps := newDeps(t)
		err := ExecPull("table", domain.NewTagSet([]string{"demo"}),
			[]string{"acme", "api"}, deps.RuntimeCLI)
		if err != nil {
			t.Fatalf("ExecPull error = %v, want nil", err)
		}
	})

	t.Run("an untagged repository is reported as untagged", func(t *testing.T) {
		t.Parallel()

		deps := newDeps(t)
		err := ExecPull("table", domain.NewTagSet([]string{"demo"}),
			[]string{"acme", "web"}, deps.RuntimeCLI)
		if err == nil {
			t.Fatal("ExecPull = nil, want the untagged repository refused")
		}
		if !strings.Contains(err.Error(), "does not carry tag") {
			t.Errorf("error = %q, want it to say the repository is not tagged", err)
		}
		if !strings.Contains(err.Error(), "web") || !strings.Contains(err.Error(), "demo") {
			t.Errorf("error = %q, want it to name both the repository and the tag", err)
		}
	})
}

// TestExecPullTagJSONCarriesTags checks that JSON includes effective tags.
func TestExecPullTagJSONCarriesTags(t *testing.T) {
	t.Parallel()

	g := tracking()
	deps := clitest.New(t, g).WithProject("acme",
		clitest.Cloned("api").Tagged("demo"))

	err := ExecPull("json", domain.NewTagSet([]string{"demo"}), []string{"acme"}, deps.RuntimeCLI)
	if err != nil {
		t.Fatalf("ExecPull error = %v, want nil", err)
	}

	repos := deps.JSONRepos("acme")
	api, ok := repos["api"]
	if !ok {
		t.Fatalf("envelope = %v, want the api repository", repos)
	}
	tags, ok := api["tags"].([]any)
	if !ok || len(tags) != 1 || tags[0] != "demo" {
		t.Errorf("api tags = %v, want [demo]", api["tags"])
	}
}
