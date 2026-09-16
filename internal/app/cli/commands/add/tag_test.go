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

// TestExecAddWritesTags checks that `--tag` writes tags on the new entry.
//
//nolint:paralleltest // addDeps calls t.Chdir, which is incompatible with t.Parallel.
func TestExecAddWritesTags(t *testing.T) {
	const src = "git@example.com:fixture/api.git"
	g := &fakeGit{remote: src}
	deps, _ := addDeps(t, g)

	err := ExecAdd(domain.NewTagSet([]string{"demo,backend"}),
		[]string{"myproj", src}, deps.RuntimeCLI)
	if err != nil {
		t.Fatalf("ExecAdd error = %v, want nil", err)
	}

	got := readConfig(t, deps.ConfigPath)
	if !strings.Contains(got, "tags:") {
		t.Fatalf("config = %q, want a tags key on the new entry", got)
	}
	for _, tag := range []string{"demo", "backend"} {
		if !strings.Contains(got, tag) {
			t.Errorf("config = %q, want it to carry tag %q", got, tag)
		}
	}
	if out := deps.Result(); !strings.Contains(out, "tagged") {
		t.Errorf("Result Output = %q, want it to report what was tagged", out)
	}
}

// TestExecAddWithoutTagsWritesNoKey checks that no `tags:` key is written
// without `--tag`.
//
//nolint:paralleltest // addDeps calls t.Chdir, which is incompatible with t.Parallel.
func TestExecAddWithoutTagsWritesNoKey(t *testing.T) {
	const src = "git@example.com:fixture/api.git"
	g := &fakeGit{remote: src}
	deps, _ := addDeps(t, g)

	if err := ExecAdd(domain.TagSet{}, []string{"myproj", src}, deps.RuntimeCLI); err != nil {
		t.Fatalf("ExecAdd error = %v, want nil", err)
	}
	if got := readConfig(t, deps.ConfigPath); strings.Contains(got, "tags:") {
		t.Errorf("config = %q, want no tags key when none was asked for", got)
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

// TestExecAddTagsAnExistingRepo checks that tagging a listed repository
// amends its entry.
func TestExecAddTagsAnExistingRepo(t *testing.T) {
	t.Parallel()

	deps, dir := existingDeps(t, "")

	err := ExecAdd(domain.NewTagSet([]string{"demo"}),
		[]string{"myproj", dir}, deps.RuntimeCLI)
	if err != nil {
		t.Fatalf("ExecAdd error = %v, want nil", err)
	}

	got := readConfig(t, deps.ConfigPath)
	if !strings.Contains(got, "demo") {
		t.Errorf("config = %q, want the existing entry tagged", got)
	}
	if strings.Count(got, "dir:") != 1 {
		t.Errorf("config = %q, want the repository listed once, not duplicated", got)
	}
	if out := deps.Result(); !strings.Contains(out, "Tagged") {
		t.Errorf("Result Output = %q, want it to report the tagging", out)
	}
}

// TestExecAddTagsARelativeDirEntry checks that an entry with a relative
// `dir:` is found and tagged.
func TestExecAddTagsARelativeDirEntry(t *testing.T) {
	t.Parallel()

	deps, dir := existingDeps(t, "api")

	err := ExecAdd(domain.NewTagSet([]string{"demo"}),
		[]string{"myproj", dir}, deps.RuntimeCLI)
	if err != nil {
		t.Fatalf("ExecAdd error = %v, want nil", err)
	}

	got := readConfig(t, deps.ConfigPath)
	if !strings.Contains(got, "demo") {
		t.Errorf("config = %q, want the relative entry tagged", got)
	}
	if strings.Count(got, "dir:") != 1 {
		t.Errorf("config = %q, want the repository listed once, not duplicated", got)
	}
	if out := deps.Result(); !strings.Contains(out, "Tagged") {
		t.Errorf("Result Output = %q, want it to report the tagging", out)
	}
}

// TestExecAddTagIsIdempotent checks that re-running `--tag` does not
// duplicate the tag.
func TestExecAddTagIsIdempotent(t *testing.T) {
	t.Parallel()

	deps, dir := existingDeps(t, "")
	tags := domain.NewTagSet([]string{"demo"})

	if err := ExecAdd(tags, []string{"myproj", dir}, deps.RuntimeCLI); err != nil {
		t.Fatalf("first ExecAdd error = %v, want nil", err)
	}

	err := ExecAdd(tags, []string{"myproj", dir}, deps.RuntimeCLI)
	if err != nil && !domain.IsWarning(err) {
		t.Fatalf("second ExecAdd error = %v, want nil or a warning that nothing changed", err)
	}
	if got := readConfig(t, deps.ConfigPath); strings.Count(got, "demo") != 1 {
		t.Errorf("config = %q, want the tag written exactly once", got)
	}
}

// taggedDeps builds dependencies over a project with entry as its only
// repository and returns the clone path.
func taggedDeps(t *testing.T, repo domain.Repository, entry string) (*clitest.Deps, string) {
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

// TestExecAddTagMatchesExistingTagsByCase checks that existing tags match
// case-insensitively.
func TestExecAddTagMatchesExistingTagsByCase(t *testing.T) {
	t.Parallel()

	deps, dir := taggedDeps(t,
		domain.Repository{Dir: "api", Src: "git@x:a/api.git", Tags: []string{"Demo"}},
		"    - dir: api\n      tags: [Demo]\n")

	err := ExecAdd(domain.NewTagSet([]string{"demo"}), []string{"myproj", dir}, deps.RuntimeCLI)
	if err != nil && !domain.IsWarning(err) {
		t.Fatalf("ExecAdd error = %v, want nil or a warning that nothing changed", err)
	}
	if got := readConfig(t, deps.ConfigPath); strings.Contains(strings.ToLower(got), "demo, demo") {
		t.Errorf("config = %q, want the tag kept once regardless of case", got)
	}
}

// TestExecAddTagsASrcOnlyEntry checks that a `src:`-only entry is found and
// tagged.
func TestExecAddTagsASrcOnlyEntry(t *testing.T) {
	t.Parallel()

	deps, dir := taggedDeps(t,
		domain.Repository{Src: "git@x:a/api.git"},
		"    - src: git@x:a/api.git\n")

	err := ExecAdd(domain.NewTagSet([]string{"demo"}), []string{"myproj", dir}, deps.RuntimeCLI)
	if err != nil {
		t.Fatalf("ExecAdd error = %v, want nil", err)
	}
	got := readConfig(t, deps.ConfigPath)
	if !strings.Contains(got, "tags: [demo]") {
		t.Errorf("config = %q, want the src-only entry tagged", got)
	}
	if strings.Count(got, "src:") != 1 {
		t.Errorf("config = %q, want the repository listed once, not duplicated", got)
	}
}
