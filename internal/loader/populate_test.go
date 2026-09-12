package loader

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

	// The token command reaching the provider is proven by a failing one:
	// its error surfaces, and construction stops before any network call.
	t.Run("provider token command is threaded from settings", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("shell fixture assumes a POSIX shell")
		}
		d := deps(&recordingCache{})
		d.Settings = domain.Settings{
			GitHub: domain.ProviderSettings{TokenCmd: "exit 1"},
		}
		p := domain.Project{
			Name:   "p",
			Source: &domain.ProviderSource{Type: "github", Search: "acme"},
		}
		err := getSource(&p, d)
		if err == nil {
			t.Fatal("getSource = nil, want token command error")
		}
		if !strings.Contains(err.Error(), "token command failed") {
			t.Errorf("error = %v, want the token command failure", err)
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

	// A non-provider repo with no derivable local home is a configuration
	// that never said where the repository lives, not a missing clone.
	t.Run("no local dir on a non-provider repo is error", func(t *testing.T) {
		p := domain.Project{Repos: []domain.Repository{{Name: "a"}}}
		computeState(ctx, &p, fakeGit{})
		r := p.Repos[0]
		if r.State != domain.RepoStateError {
			t.Errorf("state = %q, want %q", r.State, domain.RepoStateError)
		}
		if r.Reason == "" {
			t.Error("reason = empty, want an explanation naming path: and dir:")
		}
	})

	t.Run("unresolvable path is Error", func(t *testing.T) {
		// AbsPath set so the early no-local-home branch is skipped; Dir empty
		// + Src without a slash makes GetRepoAbsPath fail.
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

	t.Run("provider repo without local path is remote-only", func(t *testing.T) {
		p := domain.Project{
			Source: &domain.ProviderSource{Type: "github"},
			Repos:  []domain.Repository{{Name: "a", Src: "git@github.com:acme/a.git"}},
		}
		computeState(ctx, &p, fakeGit{})
		if got := p.Repos[0].State; got != domain.RepoStateRemoteOnly {
			t.Errorf("state = %q, want %q", got, domain.RepoStateRemoteOnly)
		}
	})

	// A relative dir: is resolved against the project path. Without one it
	// would silently resolve against the process working directory, making
	// the same config report differently depending on where gits was run.
	t.Run("relative dir without project path is error", func(t *testing.T) {
		p := domain.Project{Repos: []domain.Repository{{Name: "a", Dir: "sub/a"}}}
		computeState(ctx, &p, fakeGit{})
		r := p.Repos[0]
		if r.State != domain.RepoStateError {
			t.Errorf("state = %q, want %q", r.State, domain.RepoStateError)
		}
		if r.Reason == "" {
			t.Error("reason = empty, want an explanation naming path:")
		}
	})

	t.Run("absolute dir without project path still resolves", func(t *testing.T) {
		dir := t.TempDir()
		p := domain.Project{Repos: []domain.Repository{{Name: "a", Dir: dir, Src: "x"}}}
		computeState(ctx, &p, fakeGit{isRepo: true})
		r := p.Repos[0]
		if r.State != domain.RepoStateOK {
			t.Errorf("state = %q, want %q (reason %q)", r.State, domain.RepoStateOK, r.Reason)
		}
		if r.AbsPath != dir {
			t.Errorf("abs path = %q, want %q", r.AbsPath, dir)
		}
	})

	t.Run("uncloned repo is not-cloned without stale error reason", func(t *testing.T) {
		// The repo directory does not exist, so git.Remote must not run at
		// all: an error from it must not linger as a Reason.
		p := domain.Project{
			Path:  t.TempDir(),
			Repos: []domain.Repository{{Name: "missing", Dir: "missing"}},
		}
		computeState(ctx, &p, fakeGit{remoteErr: fmt.Errorf("boom")})
		r := p.Repos[0]
		if r.State != domain.RepoStateNotCloned {
			t.Errorf("state = %q, want %q", r.State, domain.RepoStateNotCloned)
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

	t.Run("remote lookup failure is confined to the repo that needed it", func(t *testing.T) {
		// With git missing from PATH every git.Remote call fails, so the
		// blast radius has to be one repository — not the project. A repo
		// that already carries a Src never shells out and stays ok.
		root := t.TempDir()
		needsGit := filepath.Join(root, "needs-git")
		hasSrc := filepath.Join(root, "has-src")
		for _, d := range []string{needsGit, hasSrc} {
			if err := os.MkdirAll(d, 0o755); err != nil {
				t.Fatal(err)
			}
		}
		p := domain.Project{
			Path: root,
			Repos: []domain.Repository{
				{Name: "needs-git", Dir: needsGit},
				{Name: "has-src", Dir: hasSrc, Src: "git@x:a/has-src.git"},
			},
		}
		computeState(ctx, &p, fakeGit{isRepo: true, remoteErr: fmt.Errorf("boom")})

		byName := map[string]domain.Repository{}
		for _, r := range p.Repos {
			byName[r.Name] = r
		}
		if got := byName["needs-git"].State; got != domain.RepoStateError {
			t.Errorf("needs-git state = %q, want %q", got, domain.RepoStateError)
		}
		if got := byName["has-src"].State; got != domain.RepoStateOK {
			t.Errorf("has-src state = %q, want %q (reason %q)",
				got, domain.RepoStateOK, byName["has-src"].Reason)
		}
		if got := byName["has-src"].Reason; got != "" {
			t.Errorf("has-src reason = %q, want empty", got)
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

	// A sub-project that declares no path sits at a directory named after it
	// beneath its parent, and its repositories are classified against that.
	t.Run("sub-project inherits parent path and classifies its repos", func(t *testing.T) {
		root := t.TempDir()
		subPath := filepath.Join(root, "team")
		if err := os.MkdirAll(filepath.Join(subPath, "api"), 0o750); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		p := domain.Project{
			Path: root,
			SubProjects: []domain.Project{{
				Name:  "team",
				Repos: []domain.Repository{{Name: "api", Src: "git@github.com:acme/api.git"}},
			}},
		}
		computeState(ctx, &p, fakeGit{isRepo: true})

		sub := p.SubProjects[0]
		if sub.AbsPath != subPath {
			t.Errorf("sub-project abs path = %q, want %q", sub.AbsPath, subPath)
		}
		r := sub.Repos[0]
		if r.State != domain.RepoStateOK {
			t.Errorf("state = %q, want %q (reason %q)", r.State, domain.RepoStateOK, r.Reason)
		}
		if want := filepath.Join(subPath, "api"); r.AbsPath != want {
			t.Errorf("repo abs path = %q, want %q", r.AbsPath, want)
		}
	})

	// A sub-project sits beneath its parent only when the parent has a path
	// to sit beneath. Without one there is no local home, and the sub-project
	// must not fall back to a relative path resolved against the process
	// working directory.
	t.Run("path-less parent gives sub-project no local home", func(t *testing.T) {
		p := domain.Project{
			SubProjects: []domain.Project{{
				Name:  "team",
				Repos: []domain.Repository{{Name: "api", Src: "git@github.com:acme/api.git"}},
			}},
		}
		computeState(ctx, &p, fakeGit{})

		sub := p.SubProjects[0]
		if sub.AbsPath != "" {
			t.Errorf("sub-project abs path = %q, want empty (parent declares no path:)", sub.AbsPath)
		}
		r := sub.Repos[0]
		if r.State != domain.RepoStateError {
			t.Errorf("state = %q, want %q", r.State, domain.RepoStateError)
		}
		if r.Reason == "" {
			t.Error("reason = empty, want an explanation")
		}
		if r.AbsPath != "" {
			t.Errorf("repo abs path = %q, want empty", r.AbsPath)
		}
	})

	t.Run("provider-backed sub-project without a home is remote-only", func(t *testing.T) {
		p := domain.Project{
			Source: &domain.ProviderSource{Type: "github", Search: "acme"},
			SubProjects: []domain.Project{{
				Name:  "team",
				Repos: []domain.Repository{{Name: "api", Src: "git@github.com:acme/api.git"}},
			}},
		}
		computeState(ctx, &p, fakeGit{})

		if got := p.SubProjects[0].Repos[0].State; got != domain.RepoStateRemoteOnly {
			t.Errorf("state = %q, want %q", got, domain.RepoStateRemoteOnly)
		}
	})

	t.Run("absolute dir under a path-less parent still resolves", func(t *testing.T) {
		dir := t.TempDir()
		p := domain.Project{
			SubProjects: []domain.Project{{
				Name:  "team",
				Repos: []domain.Repository{{Name: "api", Dir: dir, Src: "x"}},
			}},
		}
		computeState(ctx, &p, fakeGit{isRepo: true})

		r := p.SubProjects[0].Repos[0]
		if r.State != domain.RepoStateOK {
			t.Errorf("state = %q, want %q (reason %q)", r.State, domain.RepoStateOK, r.Reason)
		}
		if r.AbsPath != dir {
			t.Errorf("repo abs path = %q, want %q", r.AbsPath, dir)
		}
	})

	// The one genuine cross-pass dependency: path expansion writes the
	// inherited source, classification reads it back a pass later.
	t.Run("inherited provider source reaches classification", func(t *testing.T) {
		p := domain.Project{
			Source: &domain.ProviderSource{Type: "github", Search: "acme"},
			SubProjects: []domain.Project{{
				Name:  "team",
				Repos: []domain.Repository{{Name: "api", Src: "git@github.com:acme/api.git"}},
			}},
		}
		computeState(ctx, &p, fakeGit{})

		sub := p.SubProjects[0]
		if sub.Source == nil {
			t.Fatal("sub-project source = nil, want the parent's copied source")
		}
		if sub.Source.Type != "github" {
			t.Errorf("sub-project source type = %q, want %q", sub.Source.Type, "github")
		}
		if got := sub.Repos[0].Type; got != "github" {
			t.Errorf("repo type = %q, want %q (inherited source must reach classification)",
				got, "github")
		}
	})

	t.Run("ordering applies at every depth", func(t *testing.T) {
		p := domain.Project{
			SubProjects: []domain.Project{{
				Name:        "team",
				Repos:       []domain.Repository{{Name: "zzz"}, {Name: "aaa"}},
				SubProjects: []domain.Project{{Name: "b"}, {Name: "a"}},
			}},
		}
		computeState(ctx, &p, fakeGit{})

		sub := p.SubProjects[0]
		if sub.Repos[0].Name != "aaa" || sub.Repos[1].Name != "zzz" {
			t.Errorf("nested repos not sorted: %q, %q", sub.Repos[0].Name, sub.Repos[1].Name)
		}
		if sub.SubProjects[0].Name != "a" || sub.SubProjects[1].Name != "b" {
			t.Errorf("nested sub-projects not sorted: %q, %q",
				sub.SubProjects[0].Name, sub.SubProjects[1].Name)
		}
	})
}
