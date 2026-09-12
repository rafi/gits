package bulk

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cli"
	"github.com/rafi/gits/internal/cli/clitest"
	"github.com/rafi/gits/internal/types"
)

// TestLinesRendersBodyOrBareError: a result line is the padded title followed
// by the body, or — when the result failed — by the bare error in its place,
// without the repository's name and path the epilogue attaches.
func TestLinesRendersBodyOrBareError(t *testing.T) {
	t.Parallel()

	proj := domain.Project{Name: "acme", Repos: []domain.Repository{
		{Name: "api", Dir: "api", AbsPath: "/code/acme/api"},
		{Name: "web", Dir: "web", AbsPath: "/code/acme/web"},
	}}
	res := Results[string]{Project: proj, Results: []Result[string]{
		{Repo: Repo{Repository: proj.Repos[0], Project: proj, Path: "api"},
			Value: "[main <- origin/main] ok"},
		{Repo: Repo{Repository: proj.Repos[1], Project: proj, Path: "web"},
			Value: "never shown", Err: errors.New("not cloned")},
	}}

	deps := clitest.New(t, nil)
	err := Lines(res, deps.RuntimeCLI)
	if err == nil || !strings.Contains(err.Error(), "completed with errors") {
		t.Fatalf("Lines error = %v, want the failure to fail the run", err)
	}

	got := deps.Result()
	if !strings.Contains(got, "api") || !strings.Contains(got, "[main <- origin/main] ok") {
		t.Errorf("Result Output = %q, want the title and the body", got)
	}
	if !strings.Contains(got, "not cloned") || strings.Contains(got, "never shown") {
		t.Errorf("Result Output = %q, want the error to replace the body", got)
	}
	if strings.Contains(got, "/code/acme/web") {
		t.Errorf("Result Output = %q, want the bare error, not the epilogue's name and path", got)
	}
	if strings.Contains(got, "::") || strings.Contains(got, "acme") {
		t.Errorf("Result Output = %q, want no project title", got)
	}
	if diag := deps.Diagnostic(); !strings.Contains(diag, "web (/code/acme/web): not cloned") {
		t.Errorf("Diagnostic Output = %q, want the epilogue to name the repository and path", diag)
	}
}

// TestLinesSeparatesProjects: the lines of consecutive projects are separated
// by a blank line, and each project's titles are padded to its own widest.
func TestLinesSeparatesProjects(t *testing.T) {
	t.Parallel()

	root := domain.Project{Name: "root", Repos: []domain.Repository{
		{Name: "a", Dir: "a"}, {Name: "much-longer", Dir: "much-longer"},
	}}
	sub := domain.Project{Name: "sub", Repos: []domain.Repository{{Name: "s", Dir: "s"}}}
	line := func(p domain.Project, key string, i int) Result[string] {
		return Result[string]{
			Repo: Repo{
				Repository: p.Repos[i], Project: p, ProjectKey: key, Path: p.Repos[i].Dir,
			},
			Value: "BODY",
		}
	}
	res := Results[string]{Project: root, Results: []Result[string]{
		line(root, "0", 0), line(root, "0", 1), line(sub, "0/0", 0),
	}}

	deps := clitest.New(t, nil)
	if err := Lines(res, deps.RuntimeCLI); err != nil {
		t.Fatalf("Lines error = %v, want nil", err)
	}
	lines := strings.Split(strings.TrimRight(deps.Result(), "\n"), "\n")
	if len(lines) != 4 || lines[2] != "" {
		t.Fatalf("Result Output = %q, want two projects separated by a blank line", deps.Result())
	}
	if a, b := strings.Index(lines[0], "BODY"), strings.Index(lines[1], "BODY"); a != b {
		t.Errorf("bodies at columns %d and %d, want them aligned within the project", a, b)
	}
	if a, s := strings.Index(lines[0], "BODY"), strings.Index(lines[3], "BODY"); s >= a {
		t.Errorf("sub-project body at column %d, want it padded to its own width, not root's %d", s, a)
	}
}

// TestErrorsWrapsForTheEpilogue: Errors attaches the repository to every plain
// error, leaves a warning as it is, and ends with the interruption — so the
// epilogue names what failed and the run fails when cut short.
func TestErrorsWrapsForTheEpilogue(t *testing.T) {
	t.Parallel()

	repo := domain.Repository{Name: "api", AbsPath: "/code/api"}
	warning := types.NewWarning("skipped")
	res := Results[string]{
		Results: []Result[string]{
			{Repo: Repo{Repository: repo}, Err: errors.New("boom")},
			{Repo: Repo{Repository: repo}},
			{Repo: Repo{Repository: repo}, Err: warning},
		},
		Interrupted: errors.New("interrupted: 1 of 4 repositories not processed"),
	}

	errs := res.Errors()
	if len(errs) != 3 {
		t.Fatalf("Errors() = %v, want the failure, the warning and the interruption", errs)
	}
	if got := errs[0].Error(); got != "api (/code/api): boom" {
		t.Errorf("errs[0] = %q, want the repository attached", got)
	}
	if errs[1] != warning { //nolint:errorlint // identity is the assertion
		t.Errorf("errs[1] = %v, want the warning untouched", errs[1])
	}
	if !strings.Contains(errs[2].Error(), "interrupted") {
		t.Errorf("errs[2] = %v, want the interruption last", errs[2])
	}

	deps := clitest.New(t, nil)
	if err := Epilogue(res, deps.RuntimeCLI); err == nil {
		t.Fatal("Epilogue error = nil, want the failure and the interruption to fail the run")
	}
	diag := deps.Diagnostic()
	if !strings.Contains(diag, "2 errors:") || strings.Contains(diag, "skipped") {
		t.Errorf("Diagnostic Output = %q, want two counted and the warning kept out", diag)
	}
}

// TestIndentMultiline verifies the first line is left untouched and every
// continuation line is indented and prefixed with "> ", including blank lines.
func TestIndentMultiline(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"single line", "repo  ~/path", "repo  ~/path"},
		{"empty", "", ""},
		{
			"multi line",
			"repo  ~/path err\nFetching rafi\nERROR: not found",
			"repo  ~/path err\n    > Fetching rafi\n    > ERROR: not found",
		},
		{
			"blank continuation line",
			"repo err\n\nmore",
			"repo err\n    > \n    > more",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := indentMultiline(tt.in); got != tt.want {
				t.Errorf("indentMultiline(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestLinesGroupsProjectsCopiedThroughAppend: grouping must survive projects
// whose values were copied and whose Repos slices were reallocated by an
// append — the run-local ProjectKey is what identifies a node, not the
// address of a repository the project happens to hold.
func TestLinesGroupsProjectsCopiedThroughAppend(t *testing.T) {
	t.Parallel()

	root := domain.Project{Name: "root", Repos: []domain.Repository{{Name: "a", Dir: "a"}}}
	sub := domain.Project{Name: "sub", Repos: []domain.Repository{{Name: "s", Dir: "s"}}}

	// The first result holds the project as flatten saw it; the second holds
	// a copy whose Repos array was reallocated after the fact.
	grown := root
	grown.Repos = append(slices.Clone(root.Repos), domain.Repository{Name: "b", Dir: "b"})

	res := Results[string]{Project: root, Results: []Result[string]{
		{Repo: Repo{Repository: root.Repos[0], Project: root, ProjectKey: "0", Path: "a"},
			Value: "BODY"},
		{Repo: Repo{Repository: grown.Repos[1], Project: grown, ProjectKey: "0", Path: "b"},
			Value: "BODY"},
		{Repo: Repo{Repository: sub.Repos[0], Project: sub, ProjectKey: "0/0", Path: "s"},
			Value: "BODY"},
	}}

	deps := clitest.New(t, nil)
	if err := Lines(res, deps.RuntimeCLI); err != nil {
		t.Fatalf("Lines error = %v, want nil", err)
	}
	lines := strings.Split(strings.TrimRight(deps.Result(), "\n"), "\n")
	if len(lines) != 4 || lines[2] != "" {
		t.Fatalf("Result Output = %q, want the two root rows grouped and the sub-project split off",
			deps.Result())
	}
}

// TestValidateFormat: a line Bulk Command takes table and json, and says so
// instead of falling back when handed one of list's other styles.
func TestValidateFormat(t *testing.T) {
	t.Parallel()

	for _, format := range []string{FormatTable, FormatJSON} {
		if err := ValidateFormat(format); err != nil {
			t.Errorf("ValidateFormat(%q) = %v, want nil", format, err)
		}
	}
	for _, format := range []string{"name", "tree", "wide", "", "JSON"} {
		err := ValidateFormat(format)
		if err == nil {
			t.Errorf("ValidateFormat(%q) = nil, want an error", format)
			continue
		}
		if !strings.Contains(err.Error(), "table") || !strings.Contains(err.Error(), "json") {
			t.Errorf("ValidateFormat(%q) = %q, want it to name the accepted values", format, err)
		}
	}
}

// jsonFixture is a run over one project of four repositories, one in each
// condition a renderer distinguishes: a success, a documented pass-over, a
// failure, and one the state guard turned back. Its output carries terminal
// styling, as a body's does.
func jsonFixture(t *testing.T) (Results[string], *clitest.Deps) {
	t.Helper()

	deps := clitest.New(t, nil)
	proj := domain.Project{Name: "acme", Path: "/code/acme", Repos: []domain.Repository{
		{Name: "api", Dir: "api", AbsPath: "/code/acme/api", State: domain.RepoStateOK},
		{Name: "web", Dir: "web", AbsPath: "/code/acme/web", State: domain.RepoStateOK},
		{Name: "docs", Dir: "docs", AbsPath: "/code/acme/docs", State: domain.RepoStateOK},
		{Name: "gone", Dir: "gone", AbsPath: "/code/acme/gone", State: domain.RepoStateNotCloned},
	}}
	repo := func(i int) Repo {
		return Repo{Repository: proj.Repos[i], Project: proj, ProjectKey: "0", Path: proj.Repos[i].Dir}
	}
	res := Results[string]{Command: "pull", Project: proj, Results: []Result[string]{
		{Repo: repo(0), Value: deps.Theme.GitOutput.Render("Already up to date.")},
		{Repo: repo(1), Err: types.NewWarning("skipped: no upstream")},
		{Repo: repo(2), Value: "partial", Err: errors.New("boom")},
		{Repo: repo(3), Err: cli.StateError(proj.Repos[3]), Guarded: true},
	}}
	return res, deps
}

// TestJSONOutcomes: each repository's outcome nests under the command's name
// as exactly one of output, skipped or error, with the terminal styling the
// table shows stripped; a repository the guard turned back carries its state
// and no outcome, since the command never ran for it. None of it fails the
// run, and there is no epilogue: the document is the whole of the output.
func TestJSONOutcomes(t *testing.T) {
	t.Parallel()

	res, deps := jsonFixture(t)
	if err := JSON(res, deps.RuntimeCLI); err != nil {
		t.Fatalf("JSON error = %v, want nil: a repository's outcome is data", err)
	}
	if got := deps.Diagnostic(); got != "" {
		t.Errorf("Diagnostic Output = %q, want no epilogue in the json form", got)
	}
	if raw := deps.Result(); strings.Contains(raw, "\x1b") {
		t.Errorf("Result Output = %q, want no terminal styling in the document", raw)
	}

	repos := deps.JSONRepos("acme")
	want := map[string]map[string]any{
		"api":  {"output": "Already up to date."},
		"web":  {"skipped": "skipped: no upstream"},
		"docs": {"error": "boom"},
	}
	for name, outcome := range want {
		got, ok := repos[name]["pull"].(map[string]any)
		if !ok {
			t.Errorf("%s = %v, want the outcome under the command's name", name, repos[name])
			continue
		}
		if len(got) != 1 || fmt.Sprint(got) != fmt.Sprint(outcome) {
			t.Errorf("%s.pull = %v, want %v", name, got, outcome)
		}
	}
	gone := repos["gone"]
	if gone["state"] != "not-cloned" {
		t.Errorf("gone.state = %v, want not-cloned", gone["state"])
	}
	if _, found := gone["pull"]; found {
		t.Errorf("gone = %v, want no outcome for a repository the guard turned back", gone)
	}
}

// TestJSONInterrupted: an interrupted run still writes what it has, and
// still fails — the document is incomplete and nothing inside it says so.
// The repositories never started are present with their identity and state
// and no outcome.
func TestJSONInterrupted(t *testing.T) {
	t.Parallel()

	res, deps := jsonFixture(t)
	res.Results = res.Results[:1]
	res.Interrupted = errors.New("interrupted: 3 of 4 repositories not processed")

	err := JSON(res, deps.RuntimeCLI)
	if err == nil || !strings.Contains(err.Error(), "interrupted") {
		t.Fatalf("JSON error = %v, want the interruption to fail the run", err)
	}
	repos := deps.JSONRepos("acme")
	if len(repos) != 4 {
		t.Fatalf("repos = %v, want every repository of the tree, started or not", repos)
	}
	if _, ok := repos["api"]["pull"]; !ok {
		t.Errorf("api = %v, want the started repository's outcome", repos["api"])
	}
	for _, name := range []string{"web", "docs", "gone"} {
		if _, found := repos[name]["pull"]; found {
			t.Errorf("%s = %v, want no outcome for a repository never started", name, repos[name])
		}
	}
}

// TestJSONSkippedProject: a run whose named project was skipped has nothing to
// document, and says so with an empty envelope rather than a nameless node.
func TestJSONSkippedProject(t *testing.T) {
	t.Parallel()

	deps := clitest.New(t, nil)
	if err := JSON(Results[string]{Command: "clone"}, deps.RuntimeCLI); err != nil {
		t.Fatalf("JSON error = %v, want nil", err)
	}
	if got := deps.Result(); got != "{}\n" {
		t.Errorf("Result Output = %q, want an empty envelope", got)
	}
}

// TestJSONTree: sub-projects nest as the project tree does, each repository
// found by identity rather than by the order its result arrived in.
func TestJSONTree(t *testing.T) {
	t.Parallel()

	sub := domain.Project{Name: "team", Repos: []domain.Repository{
		{Name: "tools", AbsPath: "/code/acme/team/tools", State: domain.RepoStateOK},
	}}
	root := domain.Project{Name: "acme", SubProjects: []domain.Project{sub}, Repos: []domain.Repository{
		{Name: "api", AbsPath: "/code/acme/api", State: domain.RepoStateOK},
	}}
	res := Results[string]{Command: "fetch", Project: root, Results: []Result[string]{
		// Reversed from traversal order on purpose.
		{Repo: Repo{Repository: sub.Repos[0], Project: sub, ProjectKey: "0/0"}, Value: "sub"},
		{Repo: Repo{Repository: root.Repos[0], Project: root, ProjectKey: "0"}, Value: "root"},
	}}

	deps := clitest.New(t, nil)
	if err := JSON(res, deps.RuntimeCLI); err != nil {
		t.Fatalf("JSON error = %v, want nil", err)
	}
	var env map[string]struct {
		Repos []struct {
			Name  string            `json:"name"`
			Fetch map[string]string `json:"fetch"`
		} `json:"repos"`
		SubProjects []struct {
			Name  string `json:"name"`
			Repos []struct {
				Name  string            `json:"name"`
				Fetch map[string]string `json:"fetch"`
			} `json:"repos"`
		} `json:"subprojects"`
	}
	if err := json.Unmarshal([]byte(deps.Result()), &env); err != nil {
		t.Fatalf("unmarshal %q: %v", deps.Result(), err)
	}
	acme := env["acme"]
	if len(acme.Repos) != 1 || acme.Repos[0].Name != "api" || acme.Repos[0].Fetch["output"] != "root" {
		t.Errorf("acme.repos = %+v, want api with its own outcome", acme.Repos)
	}
	if len(acme.SubProjects) != 1 || acme.SubProjects[0].Name != "team" {
		t.Fatalf("acme.subprojects = %+v, want team nested", acme.SubProjects)
	}
	if team := acme.SubProjects[0].Repos; len(team) != 1 || team[0].Fetch["output"] != "sub" {
		t.Errorf("team.repos = %+v, want tools with its own outcome", team)
	}
}

// TestRenderDispatches: Render picks the renderer by format, and the exit-code
// rule follows the renderer — the same failing results fail the table and
// leave the json form at zero.
func TestRenderDispatches(t *testing.T) {
	t.Parallel()

	res, deps := jsonFixture(t)
	if err := Render(res, FormatJSON, deps.RuntimeCLI); err != nil {
		t.Errorf("Render(json) error = %v, want nil", err)
	}
	if !strings.HasPrefix(deps.Result(), "{") {
		t.Errorf("Render(json) Result Output = %q, want the document", deps.Result())
	}

	res, deps = jsonFixture(t)
	err := Render(res, FormatTable, deps.RuntimeCLI)
	if err == nil || !strings.Contains(err.Error(), "completed with errors") {
		t.Errorf("Render(table) error = %v, want the failures to fail the run", err)
	}
	if strings.HasPrefix(deps.Result(), "{") {
		t.Errorf("Render(table) Result Output = %q, want lines", deps.Result())
	}
}
