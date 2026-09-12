package loader

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cli/clitest"
	"github.com/rafi/gits/internal/git"
	"github.com/rafi/gits/internal/types"
)

// fakeGit stubs the git.Reader methods computeState relies on.
type fakeGit struct {
	git.Reader
	clitest.FakeNoWrites

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

// countingGit records how many times Remote is called, to prove ResolveSrc
// consults it once per unresolved `ok` repository and never otherwise.
type countingGit struct {
	git.Reader
	clitest.FakeNoWrites

	remote      string
	remoteErr   error
	mu          sync.Mutex
	remoteCalls int
}

func (c *countingGit) Remote(context.Context, string) (string, error) {
	c.mu.Lock()
	c.remoteCalls++
	c.mu.Unlock()
	return c.remote, c.remoteErr
}

func (c *countingGit) calls() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.remoteCalls
}

func TestIsPath(t *testing.T) {
	t.Parallel()

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
			t.Parallel()

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
//
//nolint:paralleltest // t.Chdir moves the process working directory, which is incompatible with t.Parallel.
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

	t.Run("a path argument leaves the caller's args alone", func(t *testing.T) {
		// The caller reads args[0] afterwards expecting what the user typed;
		// the resolved name is the returned map's key and Project.Name.
		args := []string{"./myrepo"}
		projs, err := GetProjects(args, deps)
		if err != nil {
			t.Fatalf("GetProjects: %v", err)
		}
		if args[0] != "./myrepo" {
			t.Errorf("args[0] = %q, want it unmodified at %q", args[0], "./myrepo")
		}
		if _, ok := projs["myrepo"]; !ok {
			t.Errorf("projects keyed %v, want the derived name %q as the key",
				projs.SortedNames(), "myrepo")
		}
		if got := ProjectName("./myrepo"); got != "myrepo" {
			t.Errorf("ProjectName(\"./myrepo\") = %q, want myrepo", got)
		}
		if got := ProjectName("acme"); got != "acme" {
			t.Errorf("ProjectName(\"acme\") = %q, want it returned as it is", got)
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
	t.Parallel()

	deps := func(c *recordingCache) types.Runtime {
		return types.Runtime{
			Ctx:   context.Background(),
			Git:   fakeGit{isRepo: true, remote: "git@x:a/b.git"},
			Cache: c,
		}
	}

	t.Run("cache hit skips the provider entirely", func(t *testing.T) {
		t.Parallel()

		// A github source without any token only works when the cache
		// serves the repos — proving no provider was constructed.
		cache := &recordingCache{hit: true, project: domain.Project{
			Repos: []domain.Repository{{Name: "cached-repo"}},
		}}
		p := domain.Project{
			Name:   "p",
			Source: &domain.ProviderSource{Type: "github", Search: "acme"},
		}
		if err := getSource(&p, deps(cache), options{}); err != nil {
			t.Fatalf("getSource: %v", err)
		}
		if len(p.Repos) != 1 || p.Repos[0].Name != "cached-repo" {
			t.Errorf("repos = %+v, want the cached repo", p.Repos)
		}
		if cache.gets != 1 || cache.saves != 0 {
			t.Errorf("cache calls = %d gets, %d saves; want 1, 0", cache.gets, cache.saves)
		}
	})

	t.Run("cache-only miss never contacts a remote provider", func(t *testing.T) {
		t.Parallel()

		// A github source with no token and a token command that would fail
		// loudly if run: proving the cache-only miss returned before any
		// provider construction, network fetch or passphrase prompt. Under the
		// default (fall-through) load this same input errors.
		cache := &recordingCache{hit: false}
		d := deps(cache)
		d.Settings = domain.Settings{
			GitHub: domain.ProviderSettings{TokenCmd: "exit 1"},
		}
		p := domain.Project{
			Name:   "p",
			Source: &domain.ProviderSource{Type: "github", Search: "acme"},
		}
		if err := getSource(&p, d, options{cacheOnly: true}); err != nil {
			t.Fatalf("getSource(cacheOnly) = %v, want nil — a miss must not reach the provider", err)
		}
		if len(p.Repos) != 0 {
			t.Errorf("repos = %+v, want none for a cold cache-only remote source", p.Repos)
		}
		if cache.gets != 1 || cache.saves != 0 {
			t.Errorf("cache calls = %d gets, %d saves; want 1, 0", cache.gets, cache.saves)
		}
	})

	t.Run("cache-only still scans a local filesystem source", func(t *testing.T) {
		t.Parallel()

		// Filesystem discovery is offline, so cache-only must not suppress it.
		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, "repo1"), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		p := domain.Project{
			Name:   "p",
			Source: &domain.ProviderSource{Type: "filesystem", Search: dir},
		}
		if err := getSource(&p, deps(&recordingCache{}), options{cacheOnly: true}); err != nil {
			t.Fatalf("getSource(cacheOnly, filesystem) = %v, want nil", err)
		}
		if len(p.Repos) == 0 {
			t.Error("repos empty, want the filesystem scan to still run under cache-only")
		}
	})

	t.Run("filesystem sources never touch the cache", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, "repo1"), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		cache := &recordingCache{}
		p := domain.Project{
			Name:   "p",
			Source: &domain.ProviderSource{Type: "filesystem", Search: dir},
		}
		if err := getSource(&p, deps(cache), options{}); err != nil {
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
		t.Parallel()

		cache := &recordingCache{}
		p := domain.Project{
			Name:   "p",
			Source: &domain.ProviderSource{Type: "filesystem", Search: t.TempDir()},
		}
		d := deps(cache)
		d.Git = fakeGit{isRepo: false} // nothing in the tree is a repo
		if err := getSource(&p, d, options{}); err == nil {
			t.Error("getSource(empty tree) = nil, want no-repositories error")
		}
	})

	// The token command reaching the provider is proven by a failing one:
	// its error surfaces, and construction stops before any network call.
	t.Run("provider token command is threaded from settings", func(t *testing.T) {
		t.Parallel()

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
		err := getSource(&p, d, options{})
		if err == nil {
			t.Fatal("getSource = nil, want token command error")
		}
		if !strings.Contains(err.Error(), "token command failed") {
			t.Errorf("error = %v, want the token command failure", err)
		}
	})

	t.Run("invalid source config errors", func(t *testing.T) {
		t.Parallel()

		p := domain.Project{
			Name:   "p",
			Source: &domain.ProviderSource{Type: "svn"},
		}
		if err := getSource(&p, deps(&recordingCache{}), options{}); err == nil {
			t.Error("getSource(invalid type) = nil, want error")
		}
	})
}

func TestComputeStateMatrix(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// A non-provider repo with no derivable local home is a configuration
	// that never said where the repository lives, not a missing clone.
	t.Run("no local dir on a non-provider repo is error", func(t *testing.T) {
		t.Parallel()

		p := domain.Project{Repos: []domain.Repository{{Name: "a"}}}
		computeState(ctx, nil, &p, fakeGit{})
		r := p.Repos[0]
		if r.State != domain.RepoStateError {
			t.Errorf("state = %q, want %q", r.State, domain.RepoStateError)
		}
		if r.Reason == "" {
			t.Error("reason = empty, want an explanation naming path: and dir:")
		}
	})

	t.Run("unresolvable path is Error", func(t *testing.T) {
		t.Parallel()

		// AbsPath set so the early no-local-home branch is skipped; Dir empty
		// + Src without a slash makes GetRepoAbsPath fail.
		p := domain.Project{Path: t.TempDir(), Repos: []domain.Repository{{Name: "a", Src: "noslash"}}}
		computeState(ctx, nil, &p, fakeGit{})
		if got := p.Repos[0].State; got != domain.RepoStateError {
			t.Errorf("state = %q, want %q", got, domain.RepoStateError)
		}
	})

	t.Run("existing non-repo dir is Error", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		p := domain.Project{Path: dir, Repos: []domain.Repository{{Name: "a", Dir: dir, Src: "x"}}}
		computeState(ctx, nil, &p, fakeGit{isRepo: false})
		r := p.Repos[0]
		if r.State != domain.RepoStateError {
			t.Errorf("state = %q, want %q", r.State, domain.RepoStateError)
		}
		if r.Reason != "Unable to load repo" {
			t.Errorf("reason = %q, want %q", r.Reason, "Unable to load repo")
		}
	})

	t.Run("existing repo dir is OK", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		p := domain.Project{Path: dir, Repos: []domain.Repository{{Name: "a", Dir: dir, Src: "x"}}}
		computeState(ctx, nil, &p, fakeGit{isRepo: true})
		if got := p.Repos[0].State; got != domain.RepoStateOK {
			t.Errorf("state = %q, want %q", got, domain.RepoStateOK)
		}
	})

	t.Run("provider repo without local path is remote-only", func(t *testing.T) {
		t.Parallel()

		p := domain.Project{
			Source: &domain.ProviderSource{Type: "github"},
			Repos:  []domain.Repository{{Name: "a", Src: "git@github.com:acme/a.git"}},
		}
		computeState(ctx, nil, &p, fakeGit{})
		if got := p.Repos[0].State; got != domain.RepoStateRemoteOnly {
			t.Errorf("state = %q, want %q", got, domain.RepoStateRemoteOnly)
		}
	})

	// A relative dir: is resolved against the project path. Without one it
	// would silently resolve against the process working directory, making
	// the same config report differently depending on where gits was run.
	t.Run("relative dir without project path is error", func(t *testing.T) {
		t.Parallel()

		p := domain.Project{Repos: []domain.Repository{{Name: "a", Dir: "sub/a"}}}
		computeState(ctx, nil, &p, fakeGit{})
		r := p.Repos[0]
		if r.State != domain.RepoStateError {
			t.Errorf("state = %q, want %q", r.State, domain.RepoStateError)
		}
		if r.Reason == "" {
			t.Error("reason = empty, want an explanation naming path:")
		}
	})

	t.Run("absolute dir without project path still resolves", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		p := domain.Project{Repos: []domain.Repository{{Name: "a", Dir: dir, Src: "x"}}}
		computeState(ctx, nil, &p, fakeGit{isRepo: true})
		r := p.Repos[0]
		if r.State != domain.RepoStateOK {
			t.Errorf("state = %q, want %q (reason %q)", r.State, domain.RepoStateOK, r.Reason)
		}
		if r.AbsPath != dir {
			t.Errorf("abs path = %q, want %q", r.AbsPath, dir)
		}
	})

	t.Run("uncloned repo is not-cloned without stale error reason", func(t *testing.T) {
		t.Parallel()

		// The repo directory does not exist, so git.Remote must not run at
		// all: an error from it must not linger as a Reason.
		p := domain.Project{
			Path:  t.TempDir(),
			Repos: []domain.Repository{{Name: "missing", Dir: "missing"}},
		}
		computeState(ctx, nil, &p, fakeGit{remoteErr: fmt.Errorf("boom")})
		r := p.Repos[0]
		if r.State != domain.RepoStateNotCloned {
			t.Errorf("state = %q, want %q", r.State, domain.RepoStateNotCloned)
		}
		if r.Reason != "" {
			t.Errorf("reason = %q, want empty (git must not run on missing dirs)", r.Reason)
		}
	})

	// Classification no longer consults git.Remote: an existing, readable
	// clone is `ok` whether or not its remote can be read. Repo Src is
	// resolved lazily by ResolveSrc, only for the commands that display it.
	t.Run("existing repo with no src is ok, remote never consulted", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		p := domain.Project{
			Path:  dir,
			Repos: []domain.Repository{{Name: "a", Dir: dir}},
		}
		// A remote error here would once have failed the repo; it must not
		// even be called now.
		computeState(ctx, nil, &p, fakeGit{isRepo: true, remoteErr: fmt.Errorf("boom")})
		r := p.Repos[0]
		if r.State != domain.RepoStateOK {
			t.Errorf("state = %q, want %q", r.State, domain.RepoStateOK)
		}
		if r.Reason != "" {
			t.Errorf("reason = %q, want empty (classification runs no git command)", r.Reason)
		}
		if r.Src != "" {
			t.Errorf("src = %q, want empty (resolution is lazy)", r.Src)
		}
	})

	t.Run("sub-projects get their own source copy", func(t *testing.T) {
		t.Parallel()

		p := domain.Project{
			Source:      &domain.ProviderSource{Type: "github", Search: "acme"},
			SubProjects: []domain.Project{{Name: "sub"}},
		}
		computeState(ctx, nil, &p, fakeGit{})
		p.SubProjects[0].Source.Search = "mutated"
		if p.Source.Search != "acme" {
			t.Errorf("parent source search = %q, want %q (sub-project must not share the pointer)",
				p.Source.Search, "acme")
		}
	})

	t.Run("repos and sub-projects sorted alphabetically", func(t *testing.T) {
		t.Parallel()

		p := domain.Project{
			Repos: []domain.Repository{{Name: "zzz"}, {Name: "aaa"}},
			SubProjects: []domain.Project{
				{Name: "b"}, {Name: "a"},
			},
		}
		computeState(ctx, nil, &p, fakeGit{})
		if p.Repos[0].Name != "aaa" || p.Repos[1].Name != "zzz" {
			t.Errorf("repos not sorted: %q, %q", p.Repos[0].Name, p.Repos[1].Name)
		}
		if p.SubProjects[0].Name != "a" || p.SubProjects[1].Name != "b" {
			t.Errorf("sub-projects not sorted: %q, %q", p.SubProjects[0].Name, p.SubProjects[1].Name)
		}
	})

	// Repositories declared with only dir: or src: have an empty Name and must
	// still sort by their display name (GetName), the same key every renderer
	// and GetRepo lookup uses — not sit in config order while named ones sort.
	t.Run("mixed name/dir/src repos sort by display name", func(t *testing.T) {
		t.Parallel()

		p := domain.Project{
			Repos: []domain.Repository{
				{Name: "delta"},
				{Dir: "bravo"},
				{Src: "git@github.com:acme/alpha.git"},
				{Name: "charlie"},
			},
		}
		computeState(ctx, nil, &p, fakeGit{})

		got := make([]string, len(p.Repos))
		for i, r := range p.Repos {
			got[i] = r.GetName()
		}
		// "alpha", not "alpha.git": the Src fallback strips the suffix so the
		// display name matches the directory derived from that same Src.
		want := []string{"alpha", "bravo", "charlie", "delta"}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("display-name order = %v, want %v", got, want)
		}
	})

	// A sub-project that declares no path sits at a directory named after it
	// beneath its parent, and its repositories are classified against that.
	t.Run("sub-project inherits parent path and classifies its repos", func(t *testing.T) {
		t.Parallel()

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
		computeState(ctx, nil, &p, fakeGit{isRepo: true})

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
		t.Parallel()

		p := domain.Project{
			SubProjects: []domain.Project{{
				Name:  "team",
				Repos: []domain.Repository{{Name: "api", Src: "git@github.com:acme/api.git"}},
			}},
		}
		computeState(ctx, nil, &p, fakeGit{})

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
		t.Parallel()

		p := domain.Project{
			Source: &domain.ProviderSource{Type: "github", Search: "acme"},
			SubProjects: []domain.Project{{
				Name:  "team",
				Repos: []domain.Repository{{Name: "api", Src: "git@github.com:acme/api.git"}},
			}},
		}
		computeState(ctx, nil, &p, fakeGit{})

		if got := p.SubProjects[0].Repos[0].State; got != domain.RepoStateRemoteOnly {
			t.Errorf("state = %q, want %q", got, domain.RepoStateRemoteOnly)
		}
	})

	t.Run("absolute dir under a path-less parent still resolves", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		p := domain.Project{
			SubProjects: []domain.Project{{
				Name:  "team",
				Repos: []domain.Repository{{Name: "api", Dir: dir, Src: "x"}},
			}},
		}
		computeState(ctx, nil, &p, fakeGit{isRepo: true})

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
		t.Parallel()

		p := domain.Project{
			Source: &domain.ProviderSource{Type: "github", Search: "acme"},
			SubProjects: []domain.Project{{
				Name:  "team",
				Repos: []domain.Repository{{Name: "api", Src: "git@github.com:acme/api.git"}},
			}},
		}
		computeState(ctx, nil, &p, fakeGit{})

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
		t.Parallel()

		p := domain.Project{
			SubProjects: []domain.Project{{
				Name:        "team",
				Repos:       []domain.Repository{{Name: "zzz"}, {Name: "aaa"}},
				SubProjects: []domain.Project{{Name: "b"}, {Name: "a"}},
			}},
		}
		computeState(ctx, nil, &p, fakeGit{})

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

// TestResolveSrc proves ResolveSrc is the lazy counterpart to classification:
// it fills in the Repo Src of every `ok` repository that has none, consulting
// git exactly once per such repository — across sub-projects too — and touches
// no repository that already has a Src or is not `ok`.
func TestResolveSrc(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("resolves ok repos without src, once each, across the tree", func(t *testing.T) {
		t.Parallel()

		g := &countingGit{remote: "git@x:a/resolved.git"}
		p := domain.Project{
			Repos: []domain.Repository{
				{Name: "needs", State: domain.RepoStateOK, AbsPath: "/a"},
				{Name: "has", State: domain.RepoStateOK, AbsPath: "/b", Src: "git@x:a/has.git"},
				{Name: "uncloned", State: domain.RepoStateNotCloned, AbsPath: "/c"},
			},
			SubProjects: []domain.Project{{
				Name: "sub",
				Repos: []domain.Repository{
					{Name: "subneeds", State: domain.RepoStateOK, AbsPath: "/d"},
				},
			}},
		}
		ResolveSrc(ctx, g, &p)

		if got := p.Repos[0].Src; got != "git@x:a/resolved.git" {
			t.Errorf("needs src = %q, want resolved", got)
		}
		if got := p.Repos[1].Src; got != "git@x:a/has.git" {
			t.Errorf("has src = %q, want its existing value untouched", got)
		}
		if got := p.Repos[2].Src; got != "" {
			t.Errorf("uncloned src = %q, want empty (only ok repos resolve)", got)
		}
		if got := p.SubProjects[0].Repos[0].Src; got != "git@x:a/resolved.git" {
			t.Errorf("sub-project repo src = %q, want resolved", got)
		}
		// Two unresolved ok repos: the top-level "needs" and the sub-project's.
		if got := g.calls(); got != 2 {
			t.Errorf("Remote calls = %d, want 2 (one per unresolved ok repo)", got)
		}
	})

	t.Run("failed lookup becomes the row's reason, not a command failure", func(t *testing.T) {
		t.Parallel()

		g := &countingGit{remoteErr: fmt.Errorf("boom")}
		p := domain.Project{
			Repos: []domain.Repository{{Name: "a", State: domain.RepoStateOK, AbsPath: "/a"}},
		}
		ResolveSrc(ctx, g, &p)

		r := p.Repos[0]
		if r.Src != "" {
			t.Errorf("src = %q, want empty on a failed lookup", r.Src)
		}
		if r.Reason != "boom" {
			t.Errorf("reason = %q, want the lookup failure recorded", r.Reason)
		}
		if r.State != domain.RepoStateOK {
			t.Errorf("state = %q, want still ok (a display detail, not a failure)", r.State)
		}
	})

	t.Run("ResolveProjectsSrc writes resolved repos back into the map", func(t *testing.T) {
		t.Parallel()

		g := &countingGit{remote: "git@x:a/resolved.git"}
		projects := domain.ProjectListKeyed{
			"acme": {Repos: []domain.Repository{
				{Name: "a", State: domain.RepoStateOK, AbsPath: "/a"},
			}},
		}
		ResolveProjectsSrc(ctx, g, projects)

		if got := projects["acme"].Repos[0].Src; got != "git@x:a/resolved.git" {
			t.Errorf("src = %q, want the resolved value written back", got)
		}
	})
}

// TestGetProjectsIssuesNoRemoteCalls pins the whole point of the lazy change:
// populating a filesystem project — the path every command runs through —
// spawns zero `git ls-remote` subprocesses. Before, classification resolved
// each repository's Repo Src eagerly, one subprocess per repository, on every
// command including ones like `list -o name` and `status` that never print it.
func TestGetProjectsIssuesNoRemoteCalls(t *testing.T) {
	t.Parallel()

	// A filesystem project with several cloned repositories: the shape a
	// large `list -o name` or `status` run walks.
	root := t.TempDir()
	for _, name := range []string{"api", "web", "cli"} {
		if err := os.MkdirAll(filepath.Join(root, name, ".git"), 0o750); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
	}

	g := &countingGit{remote: "git@x:a/b.git"}
	deps := types.Runtime{
		Ctx:   context.Background(),
		Git:   isRepoCountingGit{countingGit: g},
		Cache: &recordingCache{},
		Projects: domain.ProjectListKeyed{
			"acme": {Path: root, Source: &domain.ProviderSource{Type: "filesystem", Search: root}},
		},
	}

	projs, err := GetProjects([]string{"acme"}, deps)
	if err != nil {
		t.Fatalf("GetProjects: %v", err)
	}
	if got := len(projs["acme"].Repos); got != 3 {
		t.Fatalf("repos = %d, want 3", got)
	}
	if got := g.calls(); got != 0 {
		t.Errorf("Remote calls during population = %d, want 0 (resolution is lazy)", got)
	}
}

// isRepoCountingGit is a countingGit that also answers IsRepo by stat, so a
// filesystem walk finds the fixture repositories while Remote calls are still
// counted.
type isRepoCountingGit struct {
	*countingGit
}

func (isRepoCountingGit) IsRepo(_ context.Context, path string) bool {
	_, err := os.Stat(filepath.Join(path, ".git"))
	return err == nil
}

// TestPathlessProjectKeepsRepoIdentity pins ticket 32: a project that declares
// no `path:` must keep every configured field of its repositories. The
// path-less branch of populateProject used to rebuild each repository from
// just its Dir and Src, so a configured name, description, namespace, ID and
// URL were silently dropped — while the identical repository under a project
// that does declare a `path:` kept all of them.
func TestPathlessProjectKeepsRepoIdentity(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	configured := domain.Repository{
		ID:        "42",
		Name:      "dotfiles",
		Namespace: "rafi",
		Desc:      "my dotfiles",
		URL:       "https://example.com/acme/one",
		Dir:       dir,
		Src:       "git@example.com:acme/one.git",
	}
	p := domain.Project{Name: "acme", Repos: []domain.Repository{configured}}

	deps := types.Runtime{
		Ctx:   context.Background(),
		Git:   fakeGit{isRepo: true},
		Cache: &recordingCache{},
	}
	if err := populateProject(&p, deps, options{}); err != nil {
		t.Fatalf("populateProject: %v", err)
	}

	got := p.Repos[0]
	for _, f := range []struct{ name, got, want string }{
		{"ID", got.ID, configured.ID},
		{"Name", got.Name, configured.Name},
		{"Namespace", got.Namespace, configured.Namespace},
		{"Desc", got.Desc, configured.Desc},
		{"URL", got.URL, configured.URL},
		{"Dir", got.Dir, configured.Dir},
		{"Src", got.Src, configured.Src},
	} {
		if f.got != f.want {
			t.Errorf("%s = %q, want %q — a path-less project must not discard configured identity",
				f.name, f.got, f.want)
		}
	}
	if got.State != domain.RepoStateOK {
		t.Errorf("state = %q, want %q", got.State, domain.RepoStateOK)
	}
}

// TestPathlessProjectNameFallsBackToSrc pins the degenerate half of ticket 32:
// a repository with neither a configured name nor a `dir:` took its name from
// [filepath.Base] of an empty string — the literal ".". GetName's Src fallback
// names it instead.
func TestPathlessProjectNameFallsBackToSrc(t *testing.T) {
	t.Parallel()

	p := domain.Project{
		Name:  "acme",
		Repos: []domain.Repository{{Src: "git@example.com:acme/one.git"}},
	}
	deps := types.Runtime{
		Ctx:   context.Background(),
		Git:   fakeGit{isRepo: true},
		Cache: &recordingCache{},
	}
	if err := populateProject(&p, deps, options{}); err != nil {
		t.Fatalf("populateProject: %v", err)
	}

	if got := p.Repos[0].GetName(); got != "one" {
		t.Errorf("GetName() = %q, want %q — never the basename of an empty dir", got, "one")
	}
}

// TestSubProjectSourceIsLoaded pins the defect that a Sub-project's Provider
// Source is declared, validated and reported by `gits doctor`, but never
// asked for its repositories: populateProject only ever loaded the root
// project's source. A sub-project declaring a filesystem source of its own
// therefore came back empty, and so did one that inherited its parent's.
func TestSubProjectSourceIsLoaded(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	// The parent discovers nothing of its own; the sub-project's source is
	// the only thing that can find "svc".
	parent := filepath.Join(root, "parent")
	subPath := filepath.Join(root, "services")
	if err := os.MkdirAll(filepath.Join(parent, "app", ".git"), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(subPath, "svc", ".git"), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	p := domain.Project{
		Name: "acme",
		Path: parent,
		SubProjects: []domain.Project{{
			Name:   "backend",
			Path:   subPath,
			Source: &domain.ProviderSource{Type: "filesystem", Search: subPath},
		}},
	}
	deps := types.Runtime{
		Ctx:   context.Background(),
		Git:   isRepoCountingGit{&countingGit{}},
		Cache: &recordingCache{},
	}
	if err := populateProject(&p, deps, options{}); err != nil {
		t.Fatalf("populateProject: %v", err)
	}

	sub := p.SubProjects[0]
	if len(sub.Repos) != 1 {
		t.Fatalf("sub-project repos = %d, want 1 — its own source must be loaded", len(sub.Repos))
	}
	if got := sub.Repos[0].GetName(); got != "svc" {
		t.Errorf("repo name = %q, want %q", got, "svc")
	}
	if got := sub.Repos[0].State; got != domain.RepoStateOK {
		t.Errorf("state = %q, want %q", got, domain.RepoStateOK)
	}
}

// TestInheritedSubProjectSourceIsNotRediscovered guards the other half of the
// same rule: a Sub-project with no source of its own inherits the parent's,
// but that copy must not be discovered through. The parent's search already
// found everything beneath it, so walking it again per sub-project would
// duplicate every repository.
func TestInheritedSubProjectSourceIsNotRediscovered(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "app", ".git"), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	p := domain.Project{
		Name:        "acme",
		Path:        root,
		Source:      &domain.ProviderSource{Type: "filesystem", Search: root},
		SubProjects: []domain.Project{{Name: "team"}},
	}
	deps := types.Runtime{
		Ctx:   context.Background(),
		Git:   isRepoCountingGit{&countingGit{}},
		Cache: &recordingCache{},
	}
	if err := populateProject(&p, deps, options{}); err != nil {
		t.Fatalf("populateProject: %v", err)
	}

	if len(p.Repos) != 1 {
		t.Errorf("root repos = %d, want 1", len(p.Repos))
	}
	if got := len(p.SubProjects[0].Repos); got != 0 {
		t.Errorf("sub-project repos = %d, want 0 — an inherited source must not be re-walked", got)
	}
}

// TestPathlessGroupingProjectIsNotDiscovered pins the shape of a project that
// exists only to group Sub-projects: no `path:`, no `repos:`, no `source:`.
// The implicit filesystem default fired regardless of whether there was a
// path to search, so such a project failed validation with a message telling
// the user to set `search:` on a source they never declared.
func TestPathlessGroupingProjectIsNotDiscovered(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "svc", ".git"), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	p := domain.Project{
		Name: "acme",
		SubProjects: []domain.Project{{
			Name:   "backend",
			Path:   root,
			Source: &domain.ProviderSource{Type: "filesystem", Search: root},
		}},
	}
	deps := types.Runtime{
		Ctx:   context.Background(),
		Git:   isRepoCountingGit{&countingGit{}},
		Cache: &recordingCache{},
	}
	if err := populateProject(&p, deps, options{}); err != nil {
		t.Fatalf("populateProject: %v — a grouping project declares nothing to discover", err)
	}

	if len(p.Repos) != 0 {
		t.Errorf("root repos = %d, want 0", len(p.Repos))
	}
	if got := len(p.SubProjects[0].Repos); got != 1 {
		t.Fatalf("sub-project repos = %d, want 1", got)
	}
}
