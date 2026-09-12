package add

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rafi/gits/internal/cli/clitest"
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
