package cli

import (
	"context"
	"errors"
	"testing"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/types"
	"github.com/rafi/gits/pkg/git"
)

// fakeGit stubs the GitClient methods project population touches.
type fakeGit struct {
	git.GitClient
}

func (fakeGit) Remote(context.Context, string) (string, error) { return "git@x:a/b.git", nil }
func (fakeGit) IsRepo(context.Context, string) bool            { return true }

// stubCache is a no-op Cacher.
type stubCache struct{}

func (stubCache) Get(string, *domain.Project) (bool, error) { return false, nil }
func (stubCache) Save(string, domain.Project) error         { return nil }
func (stubCache) Flush(domain.Project) error                { return nil }

func finderDeps(t *testing.T) types.RuntimeCLI {
	t.Helper()
	return types.RuntimeCLI{Runtime: types.Runtime{
		Ctx:   context.Background(),
		Git:   fakeGit{},
		Cache: stubCache{},
		Projects: domain.ProjectListKeyed{
			"myproj": {
				Repos: []domain.Repository{
					{Dir: "~/code/alpha", Src: "git@x:acme/alpha.git"},
					{Dir: "~/code/bravo", Src: "git@x:acme/bravo.git"},
				},
			},
		},
	}}
}

// TestParseArgsNonInteractive covers every ParseArgs path that does not need
// an interactive finder.
func TestParseArgsNonInteractive(t *testing.T) {
	deps := finderDeps(t)

	t.Run("project and repo by name", func(t *testing.T) {
		proj, repo, err := ParseArgs([]string{"myproj", "alpha"}, true, deps)
		if err != nil {
			t.Fatalf("ParseArgs: %v", err)
		}
		if proj.Name != "myproj" {
			t.Errorf("project = %q, want myproj", proj.Name)
		}
		if repo == nil || repo.GetName() != "alpha" {
			t.Errorf("repo = %+v, want alpha", repo)
		}
	})

	t.Run("repo arg wins even without forced selection", func(t *testing.T) {
		_, repo, err := ParseArgs([]string{"myproj", "bravo"}, true, deps)
		if err != nil {
			t.Fatalf("ParseArgs: %v", err)
		}
		if repo == nil || repo.GetName() != "bravo" {
			t.Errorf("repo = %+v, want bravo", repo)
		}
	})

	t.Run("unknown repo errors", func(t *testing.T) {
		_, _, err := ParseArgs([]string{"myproj", "nope"}, true, deps)
		if err == nil {
			t.Fatal("ParseArgs(unknown repo) = nil, want error")
		}
	})

	t.Run("skip select without repo arg returns nil repo", func(t *testing.T) {
		proj, repo, err := ParseArgs([]string{"myproj"}, true, deps)
		if err != nil {
			t.Fatalf("ParseArgs: %v", err)
		}
		if proj.Name != "myproj" || repo != nil {
			t.Errorf("ParseArgs = (%q, %+v), want (myproj, nil repo)", proj.Name, repo)
		}
	})

	t.Run("unknown project warns", func(t *testing.T) {
		_, _, err := ParseArgs([]string{"ghost"}, true, deps)
		var warn *types.Warning
		if !errors.As(err, &warn) {
			t.Errorf("ParseArgs(ghost) error = %T (%v), want *types.Warning", err, err)
		}
	})
}
