package clone

import (
	"context"
	"strings"
	"testing"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cli"
	"github.com/rafi/gits/internal/cli/config"
	"github.com/rafi/gits/internal/types"
	"github.com/rafi/gits/pkg/git"
)

// fakeGit embeds the interface; only Clone is exercised here. The network
// success path is covered by manual demos (SPEC §5).
type fakeGit struct {
	git.GitClient
	out string
	err error
}

func (f fakeGit) Clone(context.Context, string, string) (string, error) {
	return f.out, f.err
}

func cloneDeps(g git.GitClient) types.RuntimeCLI {
	return types.RuntimeCLI{
		Theme: config.NewThemeDefault(),
		Runtime: types.Runtime{
			Ctx: context.Background(),
			Git: g,
		},
	}
}

// TestCloneRepoStateError: a repo in error state is reported via the stdout-free
// state-error path and counts as a failure.
func TestCloneRepoStateError(t *testing.T) {
	deps := cloneDeps(fakeGit{})
	repo := domain.Repository{Name: "acme", AbsPath: "/nonexistent/acme", State: domain.RepoStateError}
	project := domain.Project{Name: "p", Repos: []domain.Repository{repo}}

	res := cloneRepo(cli.NewTitleWidths(project, deps.HomeDir))(context.Background(), project, repo, deps)
	if res.Err == nil {
		t.Fatal("expected an error for a repo in error state")
	}
	if res.Line == "" {
		t.Fatal("expected a rendered line for an error-state repo")
	}
	if cli.RenderErrors([]error{res.Err}, true) == nil {
		t.Fatal("error-state repo should count as a failure")
	}
}

// TestPruneSkipped: a project with Clone=false keeps its header node but drops
// its repos and entire subtree, so the walker clones nothing under it.
func TestPruneSkipped(t *testing.T) {
	no := false
	sub := domain.Project{Name: "sub", Repos: []domain.Repository{{Name: "x"}}}
	skipped := domain.Project{
		Name:        "skip",
		Clone:       &no,
		Repos:       []domain.Repository{{Name: "y"}},
		SubProjects: []domain.Project{sub},
	}
	root := domain.Project{
		Name:        "root",
		Repos:       []domain.Repository{{Name: "z"}},
		SubProjects: []domain.Project{skipped},
	}

	pruned := pruneSkipped(root)
	if len(pruned.Repos) != 1 {
		t.Fatalf("root repos = %d, want 1", len(pruned.Repos))
	}
	if len(pruned.SubProjects) != 1 {
		t.Fatalf("subprojects = %d, want 1", len(pruned.SubProjects))
	}
	s := pruned.SubProjects[0]
	if len(s.Repos) != 0 {
		t.Fatalf("skipped project repos = %d, want 0", len(s.Repos))
	}
	if len(s.SubProjects) != 0 {
		t.Fatalf("skipped project subprojects = %d, want 0", len(s.SubProjects))
	}
	// The original tree must be left untouched (prune returns a copy).
	if len(root.SubProjects[0].Repos) != 1 {
		t.Fatal("pruneSkipped mutated the original tree")
	}
}

// TestCloneRepoAlreadyCloned: Git.Clone's ErrTargetExists sentinel maps to
// the friendly "already cloned" warning (exit 0), with no duplicate stat
// guard in the handler.
func TestCloneRepoAlreadyCloned(t *testing.T) {
	deps := cloneDeps(fakeGit{err: git.ErrTargetExists})
	repo := domain.Repository{Name: "acme", AbsPath: "/nonexistent/acme", State: domain.RepoStateNoLocal}
	project := domain.Project{Name: "p", Repos: []domain.Repository{repo}}

	res := cloneRepo(cli.NewTitleWidths(project, deps.HomeDir))(context.Background(), project, repo, deps)
	if res.Err == nil || !strings.Contains(res.Err.Error(), "already cloned") {
		t.Fatalf("want already-cloned warning, got %v", res.Err)
	}
	if !types.IsWarning(res.Err) {
		t.Fatalf("already-cloned must be a warning, got %v", res.Err)
	}
	if cli.RenderErrors([]error{res.Err}, true) != nil {
		t.Fatal("already-cloned should be a warning, not a counted error")
	}
}
