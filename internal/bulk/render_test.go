package bulk

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/rafi/gits/domain"
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
