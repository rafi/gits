package add

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/app/cli/clitest"
)

// Every test here drives ExecAdd — the command's real entry point — naming the
// project explicitly, so the interactive finder is never reached. `add` works
// on the current directory, so each test moves into a temporary one and reads
// the working directory back: it is the resolved form ExecAdd itself sees, and
// on macOS that is not the string t.TempDir returned.

// fakeGit answers the three calls ExecAdd makes, and records what it was asked
// to clone and where — the derivation from Repo Src to directory being what
// one of these tests is about. IsRepo is inherited from clitest.FakeGit, which
// reports every path a repository.
type fakeGit struct {
	clitest.FakeGit

	remote   string
	out      string
	cloneErr error

	clonedTo string
}

func (f *fakeGit) Clone(_ context.Context, _, path string) (string, error) {
	f.clonedTo = path
	return f.out, f.cloneErr
}

func (f *fakeGit) Remote(context.Context, string) (string, error) {
	return f.remote, nil
}

// configFile writes a config file holding one project with one repository, and
// returns its path. ExecAdd edits this file in place, so every assertion about
// what was added reads it back.
func configFile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := "myproj:\n  repos:\n    - dir: ~/code/existing\n      src: git@x:a/existing.git\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

// addDeps builds dependencies over a project named in the config file
// configFile writes, with the working directory moved to a temporary one.
// It returns the dependencies and that working directory as ExecAdd sees it.
func addDeps(t *testing.T, g *fakeGit) (*clitest.Deps, string) {
	t.Helper()
	// The project lists a repository of its own: a project with a Project Path
	// and nothing in it is discovered from the filesystem, which `add` refuses
	// as a provider-backed project.
	deps := clitest.New(t, g).WithProject("myproj", clitest.Cloned("existing"))
	deps.ConfigPath = configFile(t)

	t.Chdir(t.TempDir())
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return deps, cwd
}

// TestExecAddDerivesDirFromRepoSrc covers `gits add myproj <src>`: the
// repository's directory name is derived from its Repo Src — the basename
// without the `.git` suffix — and both the directory and the Repo Src are
// written into the config file.
//
//nolint:paralleltest // addDeps calls t.Chdir, which is incompatible with t.Parallel.
func TestExecAddDerivesDirFromRepoSrc(t *testing.T) {
	const src = "git@example.com:fixture/api.git"
	g := &fakeGit{remote: src}
	deps, cwd := addDeps(t, g)

	if err := ExecAdd([]string{"myproj", src}, deps.RuntimeCLI); err != nil {
		t.Fatalf("ExecAdd error = %v, want nil", err)
	}

	want := filepath.Join(cwd, "api")
	if g.clonedTo != want {
		t.Errorf("cloned into %q, want the name derived from the Repo Src, %q", g.clonedTo, want)
	}

	config, err := os.ReadFile(deps.ConfigPath)
	if err != nil {
		t.Fatalf("read config back: %v", err)
	}
	for _, line := range []string{"dir: " + want, "src: " + src} {
		if !strings.Contains(string(config), line) {
			t.Errorf("config file is missing %q:\n%s", line, config)
		}
	}
	if got := deps.Result(); !strings.Contains(got, want) || !strings.Contains(got, "myproj") {
		t.Errorf("Result Output = %q, want the added repository and its project", got)
	}
	if got := deps.Diagnostic(); got != "" {
		t.Errorf("Diagnostic Output = %q, want empty", got)
	}
}

// TestExecAddCurrentDirectory covers `gits add myproj`: with no Repo Src
// argument nothing is cloned, and the current directory is added under the
// Repo Src its Remote reports.
//
//nolint:paralleltest // addDeps calls t.Chdir, which is incompatible with t.Parallel.
func TestExecAddCurrentDirectory(t *testing.T) {
	const src = "git@example.com:fixture/here.git"
	g := &fakeGit{remote: src}
	deps, cwd := addDeps(t, g)

	if err := ExecAdd([]string{"myproj"}, deps.RuntimeCLI); err != nil {
		t.Fatalf("ExecAdd error = %v, want nil", err)
	}

	if g.clonedTo != "" {
		t.Errorf("cloned into %q, want nothing cloned without a Repo Src argument", g.clonedTo)
	}
	config, err := os.ReadFile(deps.ConfigPath)
	if err != nil {
		t.Fatalf("read config back: %v", err)
	}
	for _, line := range []string{"dir: " + cwd, "src: " + src} {
		if !strings.Contains(string(config), line) {
			t.Errorf("config file is missing %q:\n%s", line, config)
		}
	}
}

// TestExecAddCloneFailureReportsOnDiagnostic covers a failing clone: git's own
// account of the failure explains the returned error, so it belongs on
// Diagnostic Output, and nothing is written to the config file or to Result
// Output.
//
//nolint:paralleltest // addDeps calls t.Chdir, which is incompatible with t.Parallel.
func TestExecAddCloneFailureReportsOnDiagnostic(t *testing.T) {
	g := &fakeGit{out: "fatal: repository not found", cloneErr: errors.New("exit status 128")}
	deps, _ := addDeps(t, g)

	before, err := os.ReadFile(deps.ConfigPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}

	if err := ExecAdd([]string{"myproj", "git@example.com:fixture/api.git"}, deps.RuntimeCLI); err == nil {
		t.Fatal("ExecAdd error = nil, want the failed clone to fail the command")
	}

	if got := deps.Diagnostic(); !strings.Contains(got, "repository not found") {
		t.Errorf("Diagnostic Output = %q, want git's account of the failure", got)
	}
	if got := deps.Result(); got != "" {
		t.Errorf("Result Output = %q, want empty — nothing was added", got)
	}
	after, err := os.ReadFile(deps.ConfigPath)
	if err != nil {
		t.Fatalf("read config back: %v", err)
	}
	if string(after) != string(before) {
		t.Errorf("config file changed after a failed clone:\n%s", after)
	}
}

// repoDirs creates a directory per name under cwd, each of which the fake git
// reports as a repository, and returns their paths keyed by name.
func repoDirs(t *testing.T, cwd string, names ...string) map[string]string {
	t.Helper()
	paths := make(map[string]string, len(names))
	for _, name := range names {
		path := filepath.Join(cwd, name)
		if err := os.Mkdir(path, 0o750); err != nil {
			t.Fatalf("mkdir %s: %v", name, err)
		}
		paths[name] = path
	}
	return paths
}

// TestExecAddGlobAddsEveryMatch covers `gits add myproj 'backend*'`: the
// pattern is expanded by gits itself, every matching repository is written
// to the config, and each gets its own Result Output line. Nothing is cloned.
//
//nolint:paralleltest // addDeps calls t.Chdir, which is incompatible with t.Parallel.
func TestExecAddGlobAddsEveryMatch(t *testing.T) {
	g := &fakeGit{remote: "git@example.com:fixture/x.git"}
	deps, cwd := addDeps(t, g)
	dirs := repoDirs(t, cwd, "backend-api", "backend-worker", "frontend")

	if err := ExecAdd([]string{"myproj", "backend*"}, deps.RuntimeCLI); err != nil {
		t.Fatalf("ExecAdd error = %v, want nil", err)
	}

	if g.clonedTo != "" {
		t.Errorf("cloned into %q, want nothing cloned for a glob", g.clonedTo)
	}
	config, err := os.ReadFile(deps.ConfigPath)
	if err != nil {
		t.Fatalf("read config back: %v", err)
	}
	for _, name := range []string{"backend-api", "backend-worker"} {
		if !strings.Contains(string(config), "dir: "+dirs[name]) {
			t.Errorf("config file is missing %q:\n%s", dirs[name], config)
		}
		if !strings.Contains(deps.Result(), dirs[name]) {
			t.Errorf("Result Output = %q, want a line for %q", deps.Result(), dirs[name])
		}
	}
	if strings.Contains(string(config), dirs["frontend"]) {
		t.Errorf("config file lists %q, which the pattern did not match:\n%s", dirs["frontend"], config)
	}
	if got := strings.Count(deps.Result(), "Added "); got != 2 {
		t.Errorf("Result Output has %d Added lines, want 2:\n%s", got, deps.Result())
	}
}

// TestExecAddSeveralDirectories covers `gits add myproj a b`, the shape a
// shell-expanded glob arrives in: every directory is added once, in argument
// order, and one named twice is recorded once.
//
//nolint:paralleltest // addDeps calls t.Chdir, which is incompatible with t.Parallel.
func TestExecAddSeveralDirectories(t *testing.T) {
	g := &fakeGit{remote: "git@example.com:fixture/x.git"}
	deps, cwd := addDeps(t, g)
	repoDirs(t, cwd, "alpha", "bravo")

	if err := ExecAdd([]string{"myproj", "alpha", "bravo", "alpha"}, deps.RuntimeCLI); err != nil {
		t.Fatalf("ExecAdd error = %v, want nil", err)
	}

	config, err := os.ReadFile(deps.ConfigPath)
	if err != nil {
		t.Fatalf("read config back: %v", err)
	}
	if got := strings.Count(string(config), "dir: "+cwd); got != 2 {
		t.Errorf("config gained %d repositories, want 2 (alpha once, bravo once):\n%s", got, config)
	}
	if a, b := strings.Index(deps.Result(), "alpha"), strings.Index(deps.Result(), "bravo"); a < 0 || b < a {
		t.Errorf("Result Output = %q, want alpha before bravo", deps.Result())
	}
}

// TestExecAddSkipsAlreadyListed covers a target the project already lists:
// it is passed over with a note on Diagnostic Output, the others are added,
// and when nothing is left to add the command ends with a warning rather
// than a failure, and the config file is untouched.
//
//nolint:paralleltest // addDeps calls t.Chdir, which is incompatible with t.Parallel.
func TestExecAddSkipsAlreadyListed(t *testing.T) {
	g := &fakeGit{remote: "git@example.com:fixture/x.git"}
	deps, _ := addDeps(t, g)
	existing := filepath.Join(deps.Projects["myproj"].Path, "existing")

	before, err := os.ReadFile(deps.ConfigPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}

	err = ExecAdd([]string{"myproj", existing}, deps.RuntimeCLI)
	if err == nil || !domain.IsWarning(err) {
		t.Fatalf("ExecAdd error = %v, want a downgradeable warning", err)
	}
	if got := deps.Diagnostic(); !strings.Contains(got, "already in project") {
		t.Errorf("Diagnostic Output = %q, want the skip explained", got)
	}
	after, err := os.ReadFile(deps.ConfigPath)
	if err != nil {
		t.Fatalf("read config back: %v", err)
	}
	if string(after) != string(before) {
		t.Errorf("config file changed with nothing to add:\n%s", after)
	}
}

// TestExecAddRefusesUnknownTarget covers a bare word that names nothing on
// disk: it is neither a directory nor a pattern, and does not look like a
// clone URL, so it is refused before git is asked to clone it.
//
//nolint:paralleltest // addDeps calls t.Chdir, which is incompatible with t.Parallel.
func TestExecAddRefusesUnknownTarget(t *testing.T) {
	g := &fakeGit{remote: "git@example.com:fixture/x.git"}
	deps, _ := addDeps(t, g)

	err := ExecAdd([]string{"myproj", "typo"}, deps.RuntimeCLI)
	if err == nil {
		t.Fatal("ExecAdd error = nil, want a refusal")
	}
	if g.clonedTo != "" {
		t.Errorf("cloned into %q, want nothing cloned for a bare word", g.clonedTo)
	}
	if !strings.Contains(err.Error(), "typo") {
		t.Errorf("error = %q, want it to name the argument", err)
	}
}

// TestExecAddGlobWithoutRepositoriesFails covers a pattern that matches
// directories none of which are repositories, and one that matches nothing:
// both are failures naming the pattern, so a typo is not a silent success.
//
//nolint:paralleltest // addDeps calls t.Chdir, which is incompatible with t.Parallel.
func TestExecAddGlobWithoutRepositoriesFails(t *testing.T) {
	g := &fakeGit{remote: "git@example.com:fixture/x.git"}
	deps, _ := addDeps(t, g)

	err := ExecAdd([]string{"myproj", "nothing-here*"}, deps.RuntimeCLI)
	if err == nil || !strings.Contains(err.Error(), "nothing-here*") {
		t.Errorf("ExecAdd error = %v, want a failure naming the pattern", err)
	}
}

// TestExecAddRefusesDiscoveredProject covers naming a project whose
// repositories come from a Provider Source. The implicit filesystem source a
// project with only a `path:` gets is the case a user is most likely to hit,
// so the refusal points at `gits orphan` rather than at a source type they
// never wrote.
//
//nolint:paralleltest // addDeps calls t.Chdir, which is incompatible with t.Parallel.
func TestExecAddRefusesDiscoveredProject(t *testing.T) {
	g := &fakeGit{remote: "git@example.com:fixture/x.git"}
	deps, _ := addDeps(t, g)
	root := t.TempDir()
	repoDirs(t, root, "found")
	deps.Projects["walked"] = domain.Project{Path: root}

	err := ExecAdd([]string{"walked"}, deps.RuntimeCLI)
	if err == nil {
		t.Fatal("ExecAdd error = nil, want a refusal for a discovered project")
	}
	for _, want := range []string{"walked", "gits orphan"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to mention %q", err, want)
		}
	}
	if strings.Contains(err.Error(), "filesystem") {
		t.Errorf("error = %q, want the path named, not a source type the user never wrote", err)
	}
}

// TestExecAddStartsAnEmptyConfig covers the first `gits add` against a config
// file that is empty: the project and repository are written rather than the
// command failing on a document with nothing in it.
//
//nolint:paralleltest // addDeps calls t.Chdir, which is incompatible with t.Parallel.
func TestExecAddStartsAnEmptyConfig(t *testing.T) {
	const src = "git@example.com:fixture/here.git"
	g := &fakeGit{remote: src}
	deps, cwd := addDeps(t, g)
	deps.Projects = domain.ProjectListKeyed{}
	if err := os.WriteFile(deps.ConfigPath, nil, 0o644); err != nil {
		t.Fatalf("empty config: %v", err)
	}

	if err := ExecAdd([]string{"fresh"}, deps.RuntimeCLI); err != nil {
		t.Fatalf("ExecAdd error = %v, want nil", err)
	}

	config, err := os.ReadFile(deps.ConfigPath)
	if err != nil {
		t.Fatalf("read config back: %v", err)
	}
	for _, line := range []string{"fresh:", "dir: " + cwd, "src: " + src} {
		if !strings.Contains(string(config), line) {
			t.Errorf("config file is missing %q:\n%s", line, config)
		}
	}
}
