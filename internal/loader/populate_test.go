package loader

import (
	"context"
	"fmt"
	"testing"

	"github.com/rafi/gits/domain"
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
