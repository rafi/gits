package loader

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/types"
	"github.com/rafi/gits/pkg/git"
)

// fakeGit stubs the GitClient methods computeState relies on.
type fakeGit struct {
	git.GitClient
	isRepo    bool
	remote    string
	remoteErr error
}

func (f fakeGit) IsRepo(context.Context, string) bool {
	return f.isRepo
}

func (f fakeGit) Remote(context.Context, string) (string, error) {
	return f.remote, f.remoteErr
}

func TestIsPath(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{".", true},
		{"/abs/path", true},
		{"./rel", true},
		{"../rel", true},
		{"~", true},
		{"~/foo", true},
		{"myproject", false},
		{"x", false}, // single-char plain name must not panic
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := isPath(tt.in); got != tt.want {
				t.Errorf("isPath(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

// TestGetProjectsRelativePath proves path-based projects resolve relative
// arguments against the working directory: `gits status ./dir` must find the
// repo at its absolute location instead of joining the relative path onto
// itself (dir/dir) and reporting "not cloned".
func TestGetProjectsRelativePath(t *testing.T) {
	parent := t.TempDir()
	repoDir := filepath.Join(parent, "myrepo")
	if err := os.MkdirAll(filepath.Join(repoDir, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// macOS temp dirs are symlinked (/var -> /private/var); resolve so path
	// comparisons match what filepath.Abs sees from inside the directory.
	resolved, err := filepath.EvalSymlinks(repoDir)
	if err != nil {
		t.Fatalf("eval symlinks: %v", err)
	}
	t.Chdir(parent)

	deps := types.Runtime{
		Ctx:   context.Background(),
		Git:   fakeGit{isRepo: true, remote: "git@x:a/myrepo.git"},
		Cache: &recordingCache{},
	}

	t.Run("relative dir argument", func(t *testing.T) {
		projs, err := GetProjects([]string{"./myrepo"}, deps)
		if err != nil {
			t.Fatalf("GetProjects: %v", err)
		}
		if len(projs) != 1 {
			t.Fatalf("len(projects) = %d, want 1", len(projs))
		}
		for _, p := range projs {
			if len(p.Repos) != 1 {
				t.Fatalf("len(repos) = %d, want 1: %+v", len(p.Repos), p.Repos)
			}
			r := p.Repos[0]
			if got, err := filepath.EvalSymlinks(r.AbsPath); err != nil || got != resolved {
				t.Errorf("repo AbsPath = %q, want %q", r.AbsPath, resolved)
			}
			if r.State != domain.RepoStateOK {
				t.Errorf("repo state = %q, want %q", r.State, domain.RepoStateOK)
			}
		}
	})

	t.Run("dot argument names project after the directory", func(t *testing.T) {
		t.Chdir(repoDir)
		projs, err := GetProjects([]string{"."}, deps)
		if err != nil {
			t.Fatalf("GetProjects: %v", err)
		}
		if _, ok := projs["myrepo"]; !ok {
			names := []string{}
			for name := range projs {
				names = append(names, name)
			}
			t.Errorf("project names = %v, want [myrepo] (not \".\")", names)
		}
	})
}

// recordingCache is a Cacher that can serve one canned hit and records calls.
type recordingCache struct {
	hit     bool
	project domain.Project
	gets    int
	saves   int
}

func (c *recordingCache) Get(_ string, p *domain.Project) (bool, error) {
	c.gets++
	if c.hit {
		*p = c.project
		return true, nil
	}
	return false, nil
}

func (c *recordingCache) Save(string, domain.Project) error {
	c.saves++
	return nil
}

func (c *recordingCache) Flush(domain.Project) error { return nil }

func TestGetSource(t *testing.T) {
	deps := func(c *recordingCache) types.Runtime {
		return types.Runtime{
			Ctx:   context.Background(),
			Git:   fakeGit{isRepo: true, remote: "git@x:a/b.git"},
			Cache: c,
		}
	}

	t.Run("cache hit skips the provider entirely", func(t *testing.T) {
		// A github source without any token only works when the cache
		// serves the repos — proving no provider was constructed.
		cache := &recordingCache{hit: true, project: domain.Project{
			Repos: []domain.Repository{{Name: "cached-repo"}},
		}}
		p := domain.Project{
			Name:   "p",
			Source: &domain.ProviderSource{Type: "github", Search: "acme"},
		}
		if err := getSource(&p, deps(cache)); err != nil {
			t.Fatalf("getSource: %v", err)
		}
		if len(p.Repos) != 1 || p.Repos[0].Name != "cached-repo" {
			t.Errorf("repos = %+v, want the cached repo", p.Repos)
		}
		if cache.gets != 1 || cache.saves != 0 {
			t.Errorf("cache calls = %d gets, %d saves; want 1, 0", cache.gets, cache.saves)
		}
	})

	t.Run("filesystem sources never touch the cache", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, "repo1"), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		cache := &recordingCache{}
		p := domain.Project{
			Name:   "p",
			Source: &domain.ProviderSource{Type: "filesystem", Search: dir},
		}
		if err := getSource(&p, deps(cache)); err != nil {
			t.Fatalf("getSource: %v", err)
		}
		if len(p.Repos) == 0 {
			t.Error("repos empty, want filesystem scan results")
		}
		if cache.gets != 0 || cache.saves != 0 {
			t.Errorf("cache calls = %d gets, %d saves; want none", cache.gets, cache.saves)
		}
	})

	t.Run("zero repositories is an error", func(t *testing.T) {
		cache := &recordingCache{}
		p := domain.Project{
			Name:   "p",
			Source: &domain.ProviderSource{Type: "filesystem", Search: t.TempDir()},
		}
		d := deps(cache)
		d.Git = fakeGit{isRepo: false} // nothing in the tree is a repo
		if err := getSource(&p, d); err == nil {
			t.Error("getSource(empty tree) = nil, want no-repositories error")
		}
	})

	t.Run("invalid source config errors", func(t *testing.T) {
		p := domain.Project{
			Name:   "p",
			Source: &domain.ProviderSource{Type: "svn"},
		}
		if err := getSource(&p, deps(&recordingCache{})); err == nil {
			t.Error("getSource(invalid type) = nil, want error")
		}
	})
}

func TestComputeStateMatrix(t *testing.T) {
	ctx := context.Background()

	t.Run("no local dir is N/A", func(t *testing.T) {
		p := domain.Project{Repos: []domain.Repository{{Name: "a"}}}
		computeState(ctx, &p, fakeGit{})
		if got := p.Repos[0].State; got != domain.RepoStateNoLocal {
			t.Errorf("state = %q, want %q", got, domain.RepoStateNoLocal)
		}
	})

	t.Run("unresolvable path is Error", func(t *testing.T) {
		// AbsPath set so the early N/A branch is skipped; Dir empty + Src
		// without a slash makes GetRepoAbsPath fail.
		p := domain.Project{Path: t.TempDir(), Repos: []domain.Repository{{Name: "a", Src: "noslash"}}}
		computeState(ctx, &p, fakeGit{})
		if got := p.Repos[0].State; got != domain.RepoStateError {
			t.Errorf("state = %q, want %q", got, domain.RepoStateError)
		}
	})

	t.Run("existing non-repo dir is Error", func(t *testing.T) {
		dir := t.TempDir()
		p := domain.Project{Path: dir, Repos: []domain.Repository{{Name: "a", Dir: dir, Src: "x"}}}
		computeState(ctx, &p, fakeGit{isRepo: false})
		r := p.Repos[0]
		if r.State != domain.RepoStateError {
			t.Errorf("state = %q, want %q", r.State, domain.RepoStateError)
		}
		if r.Reason != "Unable to load repo" {
			t.Errorf("reason = %q, want %q", r.Reason, "Unable to load repo")
		}
	})

	t.Run("existing repo dir is OK", func(t *testing.T) {
		dir := t.TempDir()
		p := domain.Project{Path: dir, Repos: []domain.Repository{{Name: "a", Dir: dir, Src: "x"}}}
		computeState(ctx, &p, fakeGit{isRepo: true})
		if got := p.Repos[0].State; got != domain.RepoStateOK {
			t.Errorf("state = %q, want %q", got, domain.RepoStateOK)
		}
	})

	t.Run("provider repo without local path is Remote", func(t *testing.T) {
		p := domain.Project{
			Source: &domain.ProviderSource{Type: "github"},
			Repos:  []domain.Repository{{Name: "a", Src: "git@github.com:acme/a.git"}},
		}
		computeState(ctx, &p, fakeGit{})
		if got := p.Repos[0].State; got != domain.RepoStateRemote {
			t.Errorf("state = %q, want %q", got, domain.RepoStateRemote)
		}
	})

	t.Run("uncloned repo is N/A without stale error reason", func(t *testing.T) {
		// The repo directory does not exist, so git.Remote must not run at
		// all: an error from it must not linger as Reason on an N/A repo.
		p := domain.Project{
			Path:  t.TempDir(),
			Repos: []domain.Repository{{Name: "missing", Dir: "missing"}},
		}
		computeState(ctx, &p, fakeGit{remoteErr: fmt.Errorf("boom")})
		r := p.Repos[0]
		if r.State != domain.RepoStateNoLocal {
			t.Errorf("state = %q, want %q", r.State, domain.RepoStateNoLocal)
		}
		if r.Reason != "" {
			t.Errorf("reason = %q, want empty (git must not run on missing dirs)", r.Reason)
		}
	})

	t.Run("remote lookup failure on existing repo is Error", func(t *testing.T) {
		dir := t.TempDir()
		p := domain.Project{
			Path:  dir,
			Repos: []domain.Repository{{Name: "a", Dir: dir}},
		}
		computeState(ctx, &p, fakeGit{isRepo: true, remoteErr: fmt.Errorf("boom")})
		r := p.Repos[0]
		if r.State != domain.RepoStateError {
			t.Errorf("state = %q, want %q", r.State, domain.RepoStateError)
		}
		if r.Reason != "boom" {
			t.Errorf("reason = %q, want %q", r.Reason, "boom")
		}
	})

	t.Run("sub-projects get their own source copy", func(t *testing.T) {
		p := domain.Project{
			Source:      &domain.ProviderSource{Type: "github", Search: "acme"},
			SubProjects: []domain.Project{{Name: "sub"}},
		}
		computeState(ctx, &p, fakeGit{})
		p.SubProjects[0].Source.Search = "mutated"
		if p.Source.Search != "acme" {
			t.Errorf("parent source search = %q, want %q (sub-project must not share the pointer)",
				p.Source.Search, "acme")
		}
	})

	t.Run("repos and sub-projects sorted alphabetically", func(t *testing.T) {
		p := domain.Project{
			Repos: []domain.Repository{{Name: "zzz"}, {Name: "aaa"}},
			SubProjects: []domain.Project{
				{Name: "b"}, {Name: "a"},
			},
		}
		computeState(ctx, &p, fakeGit{})
		if p.Repos[0].Name != "aaa" || p.Repos[1].Name != "zzz" {
			t.Errorf("repos not sorted: %q, %q", p.Repos[0].Name, p.Repos[1].Name)
		}
		if p.SubProjects[0].Name != "a" || p.SubProjects[1].Name != "b" {
			t.Errorf("sub-projects not sorted: %q, %q", p.SubProjects[0].Name, p.SubProjects[1].Name)
		}
	})
}
