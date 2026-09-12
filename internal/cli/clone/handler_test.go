package clone

import (
	"context"
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

	res := cloneRepo(context.Background(), project, repo, deps)
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

// TestCloneRepoAlreadyCloned: an existing path yields a warning that does not
// count as a failure, preserving the exit-0 behavior.
func TestCloneRepoAlreadyCloned(t *testing.T) {
	dir := t.TempDir()
	deps := cloneDeps(fakeGit{})
	repo := domain.Repository{Name: "acme", AbsPath: dir, State: domain.RepoStateNoLocal}
	project := domain.Project{Name: "p", Repos: []domain.Repository{repo}}

	res := cloneRepo(context.Background(), project, repo, deps)
	if res.Err == nil {
		t.Fatal("expected an already-cloned warning")
	}
	if cli.RenderErrors([]error{res.Err}, true) != nil {
		t.Fatal("already-cloned should be a warning, not a counted error")
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
