package pull

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cli"
	"github.com/rafi/gits/internal/cli/config"
	"github.com/rafi/gits/internal/types"
	"github.com/rafi/gits/pkg/git"
)

// fakeGit embeds the interface; only the branch/upstream/pull queries used by
// pullRepo are implemented. Any other call panics on the nil embed, which is
// the desired guard.
type fakeGit struct {
	git.GitClient
	branch      string
	branchErr   error
	upstream    string
	upstreamErr error
	pullOut     string
	pullErr     error
}

func (f fakeGit) CurrentBranch(context.Context, string) (string, error) {
	return f.branch, f.branchErr
}

func (f fakeGit) UpstreamBranch(context.Context, string) (string, error) {
	return f.upstream, f.upstreamErr
}

func (f fakeGit) Pull(context.Context, string) (string, error) {
	return f.pullOut, f.pullErr
}

func pullDeps(g git.GitClient) types.RuntimeCLI {
	return types.RuntimeCLI{
		Theme: config.NewThemeDefault(),
		Runtime: types.Runtime{
			Ctx: context.Background(),
			Git: g,
		},
	}
}

func okRepo() domain.Repository {
	return domain.Repository{Name: "acme", AbsPath: "/tmp/acme", State: domain.RepoStateOK}
}

// TestPullRepoOK: a successful pull renders branch, upstream, and git output.
func TestPullRepoOK(t *testing.T) {
	deps := pullDeps(fakeGit{branch: "main", upstream: "origin/main", pullOut: "up to date"})
	project := domain.Project{Name: "p", Repos: []domain.Repository{okRepo()}}

	res := pullRepo(context.Background(), project, okRepo(), deps)
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	for _, want := range []string{"main", "origin/main", "up to date"} {
		if !strings.Contains(res.Line, want) {
			t.Fatalf("line %q missing %q", res.Line, want)
		}
	}
}

// TestPullRepoNotCloned: a non-OK repo is reported via the stdout-free
// state-error path and counts as a failure.
func TestPullRepoNotCloned(t *testing.T) {
	deps := pullDeps(fakeGit{})
	repo := domain.Repository{Name: "acme", AbsPath: "/tmp/acme", State: domain.RepoStateNoLocal}
	project := domain.Project{Name: "p", Repos: []domain.Repository{repo}}

	res := pullRepo(context.Background(), project, repo, deps)
	if res.Err == nil {
		t.Fatal("expected an error for a non-cloned repo")
	}
	if res.Line == "" {
		t.Fatal("expected a rendered line for a non-cloned repo")
	}
	if cli.RenderErrors([]error{res.Err}, true) == nil {
		t.Fatal("non-cloned repo should count as a failure")
	}
}

// TestPullRepoNoUpstream: an empty upstream surfaces ErrNoUpstream as a counted
// failure instead of attempting the pull.
func TestPullRepoNoUpstream(t *testing.T) {
	deps := pullDeps(fakeGit{branch: "main", upstream: ""})
	project := domain.Project{Name: "p", Repos: []domain.Repository{okRepo()}}

	res := pullRepo(context.Background(), project, okRepo(), deps)
	if res.Err == nil {
		t.Fatal("expected an error when there is no upstream")
	}
	if !strings.Contains(res.Err.Error(), "no upstream") {
		t.Fatalf("expected a no-upstream error, got %v", res.Err)
	}
	if cli.RenderErrors([]error{res.Err}, true) == nil {
		t.Fatal("no-upstream should count as a real error")
	}
}

// TestPullRepoBranchError: a failure resolving the current branch is counted.
func TestPullRepoBranchError(t *testing.T) {
	deps := pullDeps(fakeGit{branchErr: errors.New("detached HEAD")})
	project := domain.Project{Name: "p", Repos: []domain.Repository{okRepo()}}

	res := pullRepo(context.Background(), project, okRepo(), deps)
	if res.Err == nil {
		t.Fatal("expected an error when the current branch cannot be resolved")
	}
	if cli.RenderErrors([]error{res.Err}, true) == nil {
		t.Fatal("branch failure should count as a real error")
	}
}
