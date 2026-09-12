package fetch

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

// fakeGit embeds the interface so unimplemented methods are present; only Fetch
// is exercised here. Calling any other method would panic on the nil embed,
// which is the desired guard.
type fakeGit struct {
	git.GitClient
	out string
	err error
}

func (f fakeGit) Fetch(context.Context, string) (string, error) {
	return f.out, f.err
}

func fetchDeps(g git.GitClient) types.RuntimeCLI {
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

// TestFetchRepoOK: a successful fetch yields no error and includes the git
// output in the rendered line.
func TestFetchRepoOK(t *testing.T) {
	deps := fetchDeps(fakeGit{out: "up to date"})
	project := domain.Project{Name: "p", Repos: []domain.Repository{okRepo()}}

	res := fetchRepo(context.Background(), project, okRepo(), deps)
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if !strings.Contains(res.Line, "up to date") {
		t.Fatalf("line missing git output: %q", res.Line)
	}
}

// TestFetchRepoNotCloned: a non-OK repo is reported via the state-error path
// without writing to stdout, returns a counted error, and a non-empty line.
func TestFetchRepoNotCloned(t *testing.T) {
	deps := fetchDeps(fakeGit{})
	repo := domain.Repository{Name: "acme", AbsPath: "/tmp/acme", State: domain.RepoStateNoLocal}
	project := domain.Project{Name: "p", Repos: []domain.Repository{repo}}

	res := fetchRepo(context.Background(), project, repo, deps)
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

// TestFetchRepoError: a git failure surfaces as a counted error.
func TestFetchRepoError(t *testing.T) {
	deps := fetchDeps(fakeGit{err: errors.New("network down")})
	project := domain.Project{Name: "p", Repos: []domain.Repository{okRepo()}}

	res := fetchRepo(context.Background(), project, okRepo(), deps)
	if res.Err == nil {
		t.Fatal("expected an error when git fetch fails")
	}
	if cli.RenderErrors([]error{res.Err}, true) == nil {
		t.Fatal("fetch failure should count as a real error")
	}
}
