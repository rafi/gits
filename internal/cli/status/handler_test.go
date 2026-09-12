package status

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

// fakeGit stubs the status-relevant GitClient methods. Status talks only to the
// interface, so even the clean path is exercisable here.
type fakeGit struct {
	git.GitClient
	describe    string
	modified    int
	untracked   int
	branch      string
	upstream    string
	ahead       int
	behind      int
	position    string
	modifiedErr error
}

func (f fakeGit) Describe(context.Context, string) (string, error) {
	return f.describe, nil
}

func (f fakeGit) Modified(context.Context, string) (int, error) {
	return f.modified, f.modifiedErr
}

func (f fakeGit) Untracked(context.Context, string) (int, error) {
	return f.untracked, nil
}

func (f fakeGit) CurrentBranch(context.Context, string) (string, error) {
	return f.branch, nil
}

func (f fakeGit) UpstreamBranch(context.Context, string) (string, error) {
	return f.upstream, nil
}

func (f fakeGit) Diff(context.Context, string, string, string) (int, int, error) {
	return f.ahead, f.behind, nil
}

func (f fakeGit) CurrentPosition(context.Context, string) (string, error) {
	return f.position, nil
}

func statusDeps(g git.GitClient) types.RuntimeCLI {
	return types.RuntimeCLI{
		Theme:   config.NewThemeDefault(),
		HomeDir: "/home/nobody",
		Runtime: types.Runtime{
			Ctx: context.Background(),
			Git: g,
		},
	}
}

// TestStatusRepoNotCloned: a non-OK repo is reported via the stdout-free
// state-error path and counts as a failure.
func TestStatusRepoNotCloned(t *testing.T) {
	deps := statusDeps(fakeGit{})
	repo := domain.Repository{Name: "acme", AbsPath: "/tmp/acme", State: domain.RepoStateNoLocal}
	project := domain.Project{Name: "p", Repos: []domain.Repository{repo}}

	res := statusRepo(context.Background(), project, repo, deps)
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

// TestStatusRepoModifiedError: a failure counting modifications surfaces as a
// counted error.
func TestStatusRepoModifiedError(t *testing.T) {
	deps := statusDeps(fakeGit{modifiedErr: errors.New("boom")})
	repo := domain.Repository{Name: "acme", AbsPath: "/tmp/acme", State: domain.RepoStateOK}
	project := domain.Project{Name: "p", Repos: []domain.Repository{repo}}

	res := statusRepo(context.Background(), project, repo, deps)
	if res.Err == nil {
		t.Fatal("expected an error when counting modifications fails")
	}
	if cli.RenderErrors([]error{res.Err}, true) == nil {
		t.Fatal("modified failure should count as a real error")
	}
}

// TestStatusRepoClean: a clean, up-to-date repo renders its line with no error.
func TestStatusRepoClean(t *testing.T) {
	deps := statusDeps(fakeGit{
		describe: "v1.2.3",
		branch:   "main",
		upstream: "origin/main",
		position: "abc1234",
	})
	repo := domain.Repository{Name: "acme", Dir: "acme", AbsPath: "/tmp/acme", State: domain.RepoStateOK}
	project := domain.Project{Name: "p", Repos: []domain.Repository{repo}}

	res := statusRepo(context.Background(), project, repo, deps)
	if res.Err != nil {
		t.Fatalf("expected no error, got %v", res.Err)
	}
	for _, want := range []string{"acme", "✓", "v1.2.3", "abc1234"} {
		if !strings.Contains(res.Line, want) {
			t.Fatalf("line %q missing %q", res.Line, want)
		}
	}
}
