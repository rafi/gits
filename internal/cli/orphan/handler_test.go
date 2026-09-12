package orphan

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/rafi/gits/pkg/git"
)

// fakeGit stubs the GitClient methods the nested scan uses.
type fakeGit struct {
	git.GitClient
}

func (fakeGit) Remote(context.Context, string) (string, error) {
	return "git@x:a/b.git", nil
}

// TestFindNestedRepos proves the repo-scoped orphan scan finds git
// repositories embedded inside a repository's work tree, and nothing else.
func TestFindNestedRepos(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{
		".git/objects",         // the scanned repo's own metadata: skipped
		"plain/sub",            // regular source dirs: descended, not reported
		"vendor/embedded/.git", // an embedded repo: reported
		"vendor/embedded/sub",  // inside the embedded repo: not descended
	} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	repos, err := findNestedRepos(context.Background(), root, fakeGit{})
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
