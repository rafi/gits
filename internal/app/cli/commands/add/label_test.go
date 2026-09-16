package add

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/app/cli/clitest"
)

// readConfig returns the config file ExecAdd wrote.
func readConfig(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	return string(data)
}

// TestExecAddWritesLabels checks that `--label` writes labels on the new entry.
//
//nolint:paralleltest // addDeps calls t.Chdir, which is incompatible with t.Parallel.
func TestExecAddWritesLabels(t *testing.T) {
	const src = "git@example.com:fixture/api.git"
	g := &fakeGit{remote: src}
	deps, _ := addDeps(t, g)

	err := ExecAdd(domain.NewLabelSet([]string{"demo,backend"}),
		[]string{"myproj", src}, deps.RuntimeCLI)
	if err != nil {
		t.Fatalf("ExecAdd error = %v, want nil", err)
	}

	got := readConfig(t, deps.ConfigPath)
	if !strings.Contains(got, "labels:") {
		t.Fatalf("config = %q, want a labels key on the new entry", got)
	}
	for _, label := range []string{"demo", "backend"} {
		if !strings.Contains(got, label) {
			t.Errorf("config = %q, want it to carry label %q", got, label)
		}
	}
	if out := deps.Result(); !strings.Contains(out, "labeled") {
		t.Errorf("Result Output = %q, want it to report what was labeled", out)
	}
}

// TestExecAddWithoutLabelsWritesNoKey checks that no `labels:` key is written
// without `--label`.
//
//nolint:paralleltest // addDeps calls t.Chdir, which is incompatible with t.Parallel.
func TestExecAddWithoutLabelsWritesNoKey(t *testing.T) {
	const src = "git@example.com:fixture/api.git"
	g := &fakeGit{remote: src}
	deps, _ := addDeps(t, g)

	if err := ExecAdd(domain.LabelSet{}, []string{"myproj", src}, deps.RuntimeCLI); err != nil {
		t.Fatalf("ExecAdd error = %v, want nil", err)
	}
	if got := readConfig(t, deps.ConfigPath); strings.Contains(got, "labels:") {
		t.Errorf("config = %q, want no labels key when none was asked for", got)
	}
}

// existingDeps builds dependencies over a project listing one repository
// whose config entry has the given dir.
func existingDeps(t *testing.T, dir string) (*clitest.Deps, string) {
	t.Helper()

	root := t.TempDir()
	abs := filepath.Join(root, "api")
	if err := os.MkdirAll(filepath.Join(abs, ".git"), 0o750); err != nil {
		t.Fatalf("create fixture clone: %v", err)
	}
	if dir == "" {
		dir = abs
	}

	deps := clitest.New(t, &fakeGit{remote: "git@x:a/api.git"})
	deps.Projects["myproj"] = domain.Project{
		Path:  root,
		Repos: []domain.Repository{{Dir: dir, Src: "git@x:a/api.git"}},
	}

	path := filepath.Join(t.TempDir(), "config.yaml")
	content := "myproj:\n  path: " + root + "\n  repos:\n    - dir: " + dir +
		"\n      src: git@x:a/api.git\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	deps.ConfigPath = path
	return deps, abs
}

// TestExecAddLabelsAnExistingRepo checks that labeling a listed repository
// amends its entry.
func TestExecAddLabelsAnExistingRepo(t *testing.T) {
	t.Parallel()

	deps, dir := existingDeps(t, "")

	err := ExecAdd(domain.NewLabelSet([]string{"demo"}),
		[]string{"myproj", dir}, deps.RuntimeCLI)
	if err != nil {
		t.Fatalf("ExecAdd error = %v, want nil", err)
	}

	got := readConfig(t, deps.ConfigPath)
	if !strings.Contains(got, "demo") {
		t.Errorf("config = %q, want the existing entry labeled", got)
	}
	if strings.Count(got, "dir:") != 1 {
		t.Errorf("config = %q, want the repository listed once, not duplicated", got)
	}
	if out := deps.Result(); !strings.Contains(out, "Labeled") {
		t.Errorf("Result Output = %q, want it to report the labeling", out)
	}
}

// TestExecAddLabelsARelativeDirEntry checks that an entry with a relative
// `dir:` is found and labeled.
func TestExecAddLabelsARelativeDirEntry(t *testing.T) {
	t.Parallel()

	deps, dir := existingDeps(t, "api")

	err := ExecAdd(domain.NewLabelSet([]string{"demo"}),
		[]string{"myproj", dir}, deps.RuntimeCLI)
	if err != nil {
		t.Fatalf("ExecAdd error = %v, want nil", err)
	}

	got := readConfig(t, deps.ConfigPath)
	if !strings.Contains(got, "demo") {
		t.Errorf("config = %q, want the relative entry labeled", got)
	}
	if strings.Count(got, "dir:") != 1 {
		t.Errorf("config = %q, want the repository listed once, not duplicated", got)
	}
	if out := deps.Result(); !strings.Contains(out, "Labeled") {
		t.Errorf("Result Output = %q, want it to report the labeling", out)
	}
}

// TestExecAddLabelIsIdempotent checks that re-running `--label` does not
// duplicate the label.
func TestExecAddLabelIsIdempotent(t *testing.T) {
	t.Parallel()

	deps, dir := existingDeps(t, "")
	labels := domain.NewLabelSet([]string{"demo"})

	if err := ExecAdd(labels, []string{"myproj", dir}, deps.RuntimeCLI); err != nil {
		t.Fatalf("first ExecAdd error = %v, want nil", err)
	}

	err := ExecAdd(labels, []string{"myproj", dir}, deps.RuntimeCLI)
	if err != nil && !domain.IsWarning(err) {
		t.Fatalf("second ExecAdd error = %v, want nil or a warning that nothing changed", err)
	}
	if got := readConfig(t, deps.ConfigPath); strings.Count(got, "demo") != 1 {
		t.Errorf("config = %q, want the label written exactly once", got)
	}
}

// labeledDeps builds dependencies over a project with entry as its only
// repository and returns the clone path.
func labeledDeps(t *testing.T, repo domain.Repository, entry string) (*clitest.Deps, string) {
	t.Helper()

	root := t.TempDir()
	abs := filepath.Join(root, "api")
	if err := os.MkdirAll(filepath.Join(abs, ".git"), 0o750); err != nil {
		t.Fatalf("create fixture clone: %v", err)
	}

	deps := clitest.New(t, &fakeGit{remote: "git@x:a/api.git"})
	deps.Projects["myproj"] = domain.Project{Path: root, Repos: []domain.Repository{repo}}

	path := filepath.Join(t.TempDir(), "config.yaml")
	content := "myproj:\n  path: " + root + "\n  repos:\n" + entry
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	deps.ConfigPath = path
	return deps, abs
}

// TestExecAddLabelMatchesExistingLabelsByCase checks that existing labels match
// case-insensitively.
func TestExecAddLabelMatchesExistingLabelsByCase(t *testing.T) {
	t.Parallel()

	deps, dir := labeledDeps(t,
		domain.Repository{Dir: "api", Src: "git@x:a/api.git", Labels: []string{"Demo"}},
		"    - dir: api\n      labels: [Demo]\n")

	err := ExecAdd(domain.NewLabelSet([]string{"demo"}), []string{"myproj", dir}, deps.RuntimeCLI)
	if err != nil && !domain.IsWarning(err) {
		t.Fatalf("ExecAdd error = %v, want nil or a warning that nothing changed", err)
	}
	if got := readConfig(t, deps.ConfigPath); strings.Contains(strings.ToLower(got), "demo, demo") {
		t.Errorf("config = %q, want the label kept once regardless of case", got)
	}
}

// TestExecAddLabelsASrcOnlyEntry checks that a `src:`-only entry is found and
// labeled.
func TestExecAddLabelsASrcOnlyEntry(t *testing.T) {
	t.Parallel()

	deps, dir := labeledDeps(t,
		domain.Repository{Src: "git@x:a/api.git"},
		"    - src: git@x:a/api.git\n")

	err := ExecAdd(domain.NewLabelSet([]string{"demo"}), []string{"myproj", dir}, deps.RuntimeCLI)
	if err != nil {
		t.Fatalf("ExecAdd error = %v, want nil", err)
	}
	got := readConfig(t, deps.ConfigPath)
	if !strings.Contains(got, "labels: [demo]") {
		t.Errorf("config = %q, want the src-only entry labeled", got)
	}
	if strings.Count(got, "src:") != 1 {
		t.Errorf("config = %q, want the repository listed once, not duplicated", got)
	}
}
