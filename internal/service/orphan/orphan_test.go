package orphan

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/infra/git"
	coreruntime "github.com/rafi/gits/internal/runtime"
)

// fakeGit answers the one question a work-tree scan asks of git. It embeds a
// bare client rather than a CLI test helper: this package may not import the
// view layer, and the scan never reaches the other methods.
type fakeGit struct {
	git.Client
}

func (fakeGit) Remote(context.Context, string) (string, error) {
	return "git@x:a/b.git", nil
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

// TestInWorkTree proves the repo-scoped scan finds git repositories embedded
// inside a repository's work tree, and nothing else. It pins where the scan
// stops descending, which the reported list alone does not show.
func TestInWorkTree(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mkdirs(t, root,
		".git/objects",         // the scanned repo's own metadata: skipped
		"plain/sub",            // regular source dirs: descended, not reported
		"vendor/embedded/.git", // an embedded repo: reported
		"vendor/embedded/sub",  // inside the embedded repo: not descended
	)

	repos, err := InWorkTree(root, coreruntime.Runtime{Ctx: t.Context(), Git: fakeGit{}})
	if err != nil {
		t.Fatalf("InWorkTree: %v", err)
	}
	if len(repos) != 1 {
		t.Fatalf("len(repos) = %d, want 1: %+v", len(repos), repos)
	}
	want := filepath.Join(root, "vendor", "embedded")
	if repos[0].Dir != want {
		t.Errorf("repos[0].Dir = %q, want %q", repos[0].Dir, want)
	}
	if repos[0].Src != "git@x:a/b.git" {
		t.Errorf("repos[0].Src = %q, want the clone's remote", repos[0].Src)
	}
}

// TestInProjectNeedsPath proves a Project with no path is refused rather than
// scanned: every repository under it carries an absolute path of its own, so
// there is no directory the scan could walk.
func TestInProjectNeedsPath(t *testing.T) {
	t.Parallel()

	_, err := InProject(domain.Project{Name: "acme"}, coreruntime.Runtime{Ctx: t.Context(), Git: fakeGit{}})
	if err == nil {
		t.Fatal("InProject error = nil, want a pathless project to be refused")
	}
}
