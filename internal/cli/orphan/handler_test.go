package orphan

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rafi/gits/internal/cli/clitest"
)

// The entry-point tests here drive ExecOrphan with explicit arguments — the
// project alone for the project-wide scan, the project and a repository for
// the work-tree scan — so the interactive finder is never reached.

// fakeGit answers the two questions an orphan scan asks of git. IsRepo is
// overridden rather than inherited from clitest.FakeGit because this command
// is the one that asks it about directories nobody declared: the scan finds a
// repository by asking, so a client that says yes to everything would report
// every directory it walked.
type fakeGit struct {
	clitest.FakeGit

	remote string
}

func (fakeGit) IsRepo(_ context.Context, path string) bool {
	_, err := os.Stat(filepath.Join(path, ".git"))
	return err == nil
}

func (f fakeGit) Remote(context.Context, string) (string, error) {
	if f.remote == "" {
		return "git@x:a/b.git", nil
	}
	return f.remote, nil
}

// mkdirs creates every path relative to root, parents included.
func mkdirs(t *testing.T, root string, paths ...string) {
	t.Helper()
	for _, dir := range paths {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o750); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
}

// TestExecOrphanNestedInRepository covers `gits orphan acme api`: the scan is
// scoped to that repository's work tree and reports the git repository nested
// inside it, which no Project declares.
func TestExecOrphanNestedInRepository(t *testing.T) {
	t.Parallel()

	const src = "git@x:acme/embedded.git"
	deps := clitest.New(t, fakeGit{remote: src}).WithProject("acme", clitest.Cloned("api"))

	repoRoot := filepath.Join(deps.Projects["acme"].Path, "api")
	mkdirs(t, repoRoot,
		".git/objects",         // the scanned repository's own metadata
		"plain/sub",            // regular source directories
		"vendor/embedded/.git", // the nested repository, reported
	)

	if err := ExecOrphan([]string{"acme", "api"}, deps.RuntimeCLI); err != nil {
		t.Fatalf("ExecOrphan error = %v, want nil", err)
	}

	got := deps.Result()
	for _, want := range []string{"acme", filepath.Join(repoRoot, "vendor", "embedded"), src} {
		if !strings.Contains(got, want) {
			t.Errorf("Result Output = %q, want it to contain %q", got, want)
		}
	}
	if strings.Contains(got, filepath.Join(repoRoot, "plain")) {
		t.Errorf("Result Output = %q, want nothing about a directory that is no repository", got)
	}
	if got := deps.Diagnostic(); got != "" {
		t.Errorf("Diagnostic Output = %q, want empty", got)
	}
}

// TestExecOrphanUndeclaredInProject covers `gits orphan acme`: the scan walks
// the Project Path and reports the repository sitting there that the project
// never declared, while the one it did declare is passed over.
func TestExecOrphanUndeclaredInProject(t *testing.T) {
	t.Parallel()

	deps := clitest.New(t, fakeGit{}).WithProject("acme", clitest.Cloned("api"))

	root := deps.Projects["acme"].Path
	mkdirs(t, root, "api/.git", "stray/.git")

	if err := ExecOrphan([]string{"acme"}, deps.RuntimeCLI); err != nil {
		t.Fatalf("ExecOrphan error = %v, want nil", err)
	}

	got := deps.Result()
	if want := filepath.Join(root, "stray"); !strings.Contains(got, want) {
		t.Errorf("Result Output = %q, want it to contain the undeclared repository %q", got, want)
	}
	if want := filepath.Join(root, "api"); strings.Contains(got, want) {
		t.Errorf("Result Output = %q, want nothing about the declared repository %q", got, want)
	}
}

// TestExecOrphanAbortsOnNonOKState covers the state guard on the work-tree
// scan: there is no tree to walk, so the Repository's own Reason aborts the
// command on Diagnostic Output.
func TestExecOrphanAbortsOnNonOKState(t *testing.T) {
	t.Parallel()

	deps := clitest.New(t, fakeGit{}).WithProject("acme", clitest.Broken("bad"))

	err := ExecOrphan([]string{"acme", "bad"}, deps.RuntimeCLI)
	if err == nil {
		t.Fatal("ExecOrphan error = nil, want the state to abort the command")
	}
	if want := clitest.BrokenReason + "\n"; deps.Diagnostic() != want {
		t.Errorf("Diagnostic Output = %q, want exactly %q", deps.Diagnostic(), want)
	}
	if got := deps.Result(); got != "" {
		t.Errorf("Result Output = %q, want empty", got)
	}
}

// TestFindNestedRepos proves the repo-scoped orphan scan finds git
// repositories embedded inside a repository's work tree, and nothing else. It
// stays alongside the entry-point test above because it pins where the scan
// stops descending, which the reported list alone does not show.
func TestFindNestedRepos(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mkdirs(t, root,
		".git/objects",         // the scanned repo's own metadata: skipped
		"plain/sub",            // regular source dirs: descended, not reported
		"vendor/embedded/.git", // an embedded repo: reported
		"vendor/embedded/sub",  // inside the embedded repo: not descended
	)

	repos, err := findNestedRepos(t.Context(), root, fakeGit{})
	if err != nil {
		t.Fatalf("findNestedRepos: %v", err)
	}
	if len(repos) != 1 {
		t.Fatalf("len(repos) = %d, want 1: %+v", len(repos), repos)
	}
	want := filepath.Join(root, "vendor", "embedded")
	if repos[0].Dir != want {
		t.Errorf("repos[0].Dir = %q, want %q", repos[0].Dir, want)
	}
}
