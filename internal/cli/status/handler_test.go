package status

import (
	"context"
	"errors"
	"testing"
	"time"

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
	describe       string
	snap           git.Snapshot
	snapErr        error
	workingDiff    git.DiffStat
	workingDiffErr error
	fallbackRef    string
	ahead          int
	behind         int
	diffErr        error
	head           git.Head
	headErr        error
}

func (f fakeGit) Describe(context.Context, string) (string, error) {
	return f.describe, nil
}

func (f fakeGit) Snapshot(context.Context, string) (git.Snapshot, error) {
	return f.snap, f.snapErr
}

func (f fakeGit) WorkingDiff(context.Context, string) (git.DiffStat, error) {
	return f.workingDiff, f.workingDiffErr
}

func (f fakeGit) FallbackRef(context.Context, string, string) string {
	return f.fallbackRef
}

func (f fakeGit) Diff(context.Context, string, string, string) (int, int, error) {
	return f.ahead, f.behind, f.diffErr
}

func (f fakeGit) HeadInfo(context.Context, string) (git.Head, error) {
	return f.head, f.headErr
}

func statusDeps(g git.GitClient) types.RuntimeCLI {
	settings := domain.Settings{}
	settings.Icons.ApplyDefaults()
	return types.RuntimeCLI{
		Theme:   config.NewThemeDefault(),
		HomeDir: "/home/nobody",
		Runtime: types.Runtime{
			Ctx:      context.Background(),
			Git:      g,
			Settings: settings,
		},
	}
}

// TestStatusRepoNotCloned: a non-OK repo is reported via the stdout-free
// state-error path and counts as a failure.
func TestStatusRepoNotCloned(t *testing.T) {
	deps := statusDeps(fakeGit{})
	repo := domain.Repository{Name: "acme", AbsPath: "/tmp/acme", State: domain.RepoStateNoLocal}
	project := domain.Project{Name: "p", Repos: []domain.Repository{repo}}

	res := statusRepo(Options{})(context.Background(), project, repo, deps)
	if res.Err == nil {
		t.Fatal("expected an error for a non-cloned repo")
	}
	st, ok := res.Payload.(*repoStatus)
	if !ok {
		t.Fatalf("expected *repoStatus payload, got %T", res.Payload)
	}
	if st.err == nil || st.message == "" {
		t.Fatalf("expected populated error row, got err=%v message=%q", st.err, st.message)
	}
	if cli.RenderErrors([]error{res.Err}, true) == nil {
		t.Fatal("non-cloned repo should count as a failure")
	}
}

// TestStatusRepoProbeError: a failure reading the work tree surfaces as a
// counted error with the reason in the row message.
func TestStatusRepoProbeError(t *testing.T) {
	deps := statusDeps(fakeGit{snapErr: errors.New("boom")})
	repo := domain.Repository{Name: "acme", AbsPath: "/tmp/acme", State: domain.RepoStateOK}
	project := domain.Project{Name: "p", Repos: []domain.Repository{repo}}

	res := statusRepo(Options{})(context.Background(), project, repo, deps)
	if res.Err == nil {
		t.Fatal("expected an error when reading the work tree fails")
	}
	st := res.Payload.(*repoStatus)
	if st.message != "boom" {
		t.Errorf("message = %q, want %q", st.message, "boom")
	}
	if cli.RenderErrors([]error{res.Err}, true) == nil {
		t.Fatal("work-tree failure should count as a real error")
	}
}

// TestStatusRepoDirty: work-tree counts and upstream divergence land in the
// structured payload.
func TestStatusRepoDirty(t *testing.T) {
	when := time.Now().Add(-2 * time.Hour)
	deps := statusDeps(fakeGit{
		describe: "v1.2.3",
		snap: git.Snapshot{
			Branch:      "main",
			HasUpstream: true,
			Ahead:       3,
			Behind:      1,
			WorkTree:    git.WorkTree{Staged: 1, Unstaged: 122, Untracked: 4567},
		},
		head: git.Head{Hash: "abc12345", Subject: "Add feature", Time: when},
	})
	repo := domain.Repository{Name: "acme", Dir: "acme", AbsPath: "/tmp/acme", State: domain.RepoStateOK}
	project := domain.Project{Name: "p", Repos: []domain.Repository{repo}}

	res := statusRepo(Options{})(context.Background(), project, repo, deps)
	if res.Err != nil {
		t.Fatalf("expected no error, got %v", res.Err)
	}
	st := res.Payload.(*repoStatus)
	if st.staged != 1 || st.unstaged != 122 || st.untracked != 4567 {
		t.Errorf("work tree = %d/%d/%d, want 1/122/4567",
			st.staged, st.unstaged, st.untracked)
	}
	if st.ahead != 3 || st.behind != 1 || st.noUpstream {
		t.Errorf("divergence = %d/%d noUpstream=%v, want 3/1 false",
			st.ahead, st.behind, st.noUpstream)
	}
	if !st.changed() {
		t.Error("changed() = false for a dirty work tree")
	}
	if st.branch != "main" || st.version != "v1.2.3" ||
		st.commit != "abc12345" || st.message != "Add feature" {
		t.Errorf("metadata = %q %q %q %q", st.branch, st.version, st.commit, st.message)
	}
}

// TestStatusRepoClean: a clean, up-to-date repo yields a zeroed payload with
// no error.
func TestStatusRepoClean(t *testing.T) {
	deps := statusDeps(fakeGit{
		describe: "v1.2.3",
		snap:     git.Snapshot{Branch: "main", HasUpstream: true},
		head:     git.Head{Hash: "abc12345", Subject: "Initial commit", Time: time.Now()},
	})
	repo := domain.Repository{Name: "acme", Dir: "acme", AbsPath: "/tmp/acme", State: domain.RepoStateOK}
	project := domain.Project{Name: "p", Repos: []domain.Repository{repo}}

	res := statusRepo(Options{})(context.Background(), project, repo, deps)
	if res.Err != nil {
		t.Fatalf("expected no error, got %v", res.Err)
	}
	st := res.Payload.(*repoStatus)
	if st.changed() || st.ahead != 0 || st.behind != 0 || st.err != nil {
		t.Errorf("expected clean status, got %+v", st)
	}
	if st.title != "acme" {
		t.Errorf("title = %q, want %q", st.title, "acme")
	}
}

// TestStatusRepoStat: line diffs are probed only with --stat, and a probe
// failure (e.g. unborn HEAD) leaves the counts blank without failing the row.
func TestStatusRepoStat(t *testing.T) {
	fake := fakeGit{
		workingDiff: git.DiffStat{Added: 27, Deleted: 8},
		snap:        git.Snapshot{Branch: "main", HasUpstream: true},
	}
	repo := domain.Repository{Name: "acme", AbsPath: "/tmp/acme", State: domain.RepoStateOK}
	project := domain.Project{Name: "p", Repos: []domain.Repository{repo}}

	res := statusRepo(Options{Stat: true})(context.Background(), project, repo, statusDeps(fake))
	st := res.Payload.(*repoStatus)
	if st.added != 27 || st.deleted != 8 {
		t.Errorf("line diffs = +%d -%d, want +27 -8", st.added, st.deleted)
	}

	res = statusRepo(Options{})(context.Background(), project, repo, statusDeps(fake))
	st = res.Payload.(*repoStatus)
	if st.added != 0 || st.deleted != 0 {
		t.Errorf("line diffs probed without --stat: +%d -%d", st.added, st.deleted)
	}

	fake.workingDiffErr = errors.New("unborn HEAD")
	res = statusRepo(Options{Stat: true})(context.Background(), project, repo, statusDeps(fake))
	st = res.Payload.(*repoStatus)
	if res.Err != nil || st.added != 0 || st.deleted != 0 {
		t.Errorf("diff probe failure should be tolerated, got err=%v +%d -%d",
			res.Err, st.added, st.deleted)
	}
}

// TestStatusRepoNoUpstream: a diff failure flags noUpstream instead of failing
// the row.
func TestStatusRepoNoUpstream(t *testing.T) {
	deps := statusDeps(fakeGit{
		snap:        git.Snapshot{Branch: "main"},
		fallbackRef: "origin/main",
		diffErr:     errors.New("unknown revision"),
	})
	repo := domain.Repository{Name: "acme", AbsPath: "/tmp/acme", State: domain.RepoStateOK}
	project := domain.Project{Name: "p", Repos: []domain.Repository{repo}}

	res := statusRepo(Options{})(context.Background(), project, repo, deps)
	if res.Err != nil {
		t.Fatalf("expected no error, got %v", res.Err)
	}
	st := res.Payload.(*repoStatus)
	if !st.noUpstream {
		t.Error("expected noUpstream to be set when diff fails")
	}
}

// TestStatusRepoFallbackRef: with no upstream configured but a matching
// branch on a (possibly non-origin) remote, real ahead/behind counts show
// instead of N/A.
func TestStatusRepoFallbackRef(t *testing.T) {
	deps := statusDeps(fakeGit{
		snap:        git.Snapshot{Branch: "main"},
		fallbackRef: "upstream/main",
		ahead:       2,
		behind:      1,
	})
	repo := domain.Repository{Name: "acme", AbsPath: "/tmp/acme", State: domain.RepoStateOK}
	project := domain.Project{Name: "p", Repos: []domain.Repository{repo}}

	res := statusRepo(Options{})(context.Background(), project, repo, deps)
	if res.Err != nil {
		t.Fatalf("expected no error, got %v", res.Err)
	}
	st := res.Payload.(*repoStatus)
	if st.noUpstream || st.ahead != 2 || st.behind != 1 {
		t.Errorf("fallback divergence = %d/%d noUpstream=%v, want 2/1 false",
			st.ahead, st.behind, st.noUpstream)
	}
}

// TestStatusRepoNoRemoteBranch: no upstream and no matching remote branch
// anywhere leaves the row N/A.
func TestStatusRepoNoRemoteBranch(t *testing.T) {
	deps := statusDeps(fakeGit{snap: git.Snapshot{Branch: "main"}})
	repo := domain.Repository{Name: "acme", AbsPath: "/tmp/acme", State: domain.RepoStateOK}
	project := domain.Project{Name: "p", Repos: []domain.Repository{repo}}

	res := statusRepo(Options{})(context.Background(), project, repo, deps)
	st := res.Payload.(*repoStatus)
	if !st.noUpstream {
		t.Error("expected noUpstream when no remote has the branch")
	}
}
