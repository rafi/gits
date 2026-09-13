package status

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/format"
	"github.com/rafi/gits/internal/git"
	"github.com/rafi/gits/internal/service"
	"github.com/rafi/gits/internal/service/run"
)

// fakeGit stubs the status-relevant git methods. The probe only reads, and
// the embedded interface is nil, so any call it does not answer here panics
// rather than passing quietly.
type fakeGit struct {
	git.Client

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

// statusDeps builds the runtime the probe reads from: it writes no output,
// so only the git client matters here.
func statusDeps(t *testing.T, g git.Client) service.Runtime {
	t.Helper()
	return service.Runtime{Ctx: t.Context(), Git: g}
}

// probe drives the probe for one repository, assembling the bundled argument
// the engine hands a body: the repository, its owning project and its display
// path.
func probe(
	t *testing.T,
	rt service.Runtime,
	p Probe,
	project domain.Project,
	repo domain.Repository,
) (*Report, error) {
	t.Helper()
	return p.Repo(t.Context(), run.Repo{
		Repository: repo,
		Project:    project,
		Path:       format.RepoRelPath(project, repo, rt.HomeDir),
	}, rt)
}

// TestStatusRepoProbeError: a failure reading the work tree surfaces as a
// counted error, carried on the row for its message cell.
func TestStatusRepoProbeError(t *testing.T) {
	t.Parallel()

	deps := statusDeps(t, fakeGit{snapErr: errors.New("boom")})
	repo := domain.Repository{Name: "acme", AbsPath: "/tmp/acme", State: domain.RepoStateOK}
	project := domain.Project{Name: "p", Repos: []domain.Repository{repo}}

	st, err := probe(t, deps, Probe{}, project, repo)
	if err == nil {
		t.Fatal("expected an error when reading the work tree fails")
	}
	if st.Err == nil || st.Err.Error() != "boom" {
		t.Errorf("err = %v, want %q", st.Err, "boom")
	}
	if domain.IsWarning(err) {
		t.Fatal("work-tree failure should count as a real error, not a warning")
	}
}

// TestStatusRepoDirty: work-tree counts and upstream divergence land in the
// structured payload.
func TestStatusRepoDirty(t *testing.T) {
	t.Parallel()

	when := time.Now().Add(-2 * time.Hour)
	deps := statusDeps(t, fakeGit{
		snap: git.Snapshot{
			Branch:   "main",
			Tracking: true,
			Ahead:    3,
			Behind:   1,
			WorkTree: git.WorkTree{Staged: 1, Unstaged: 122, Untracked: 4567},
		},
		head: git.Head{Hash: "abc12345", Subject: "Add feature", Time: when, Describe: "v1.2.3"},
	})
	repo := domain.Repository{Name: "acme", Dir: "acme", AbsPath: "/tmp/acme", State: domain.RepoStateOK}
	project := domain.Project{Name: "p", Repos: []domain.Repository{repo}}

	st, err := probe(t, deps, Probe{}, project, repo)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if st.Staged != 1 || st.Unstaged != 122 || st.Untracked != 4567 {
		t.Errorf("work tree = %d/%d/%d, want 1/122/4567",
			st.Staged, st.Unstaged, st.Untracked)
	}
	if st.Ahead != 3 || st.Behind != 1 || !st.Compared {
		t.Errorf("divergence = %d/%d compared=%v, want 3/1 true",
			st.Ahead, st.Behind, st.Compared)
	}
	if !st.Changed() {
		t.Error("changed() = false for a dirty work tree")
	}
	if st.Branch != "main" || st.Version != "v1.2.3" ||
		st.Head.Hash != "abc12345" || st.Head.Subject != "Add feature" {
		t.Errorf("metadata = %q %q %+v", st.Branch, st.Version, st.Head)
	}
}

// TestStatusRepoClean: a clean, up-to-date repo yields a zeroed payload with
// no error.
func TestStatusRepoClean(t *testing.T) {
	t.Parallel()

	deps := statusDeps(t, fakeGit{
		snap: git.Snapshot{Branch: "main", Tracking: true},
		head: git.Head{Hash: "abc12345", Subject: "Initial commit", Time: time.Now()},
	})
	repo := domain.Repository{Name: "acme", Dir: "acme", AbsPath: "/tmp/acme", State: domain.RepoStateOK}
	project := domain.Project{Name: "p", Repos: []domain.Repository{repo}}

	st, err := probe(t, deps, Probe{}, project, repo)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if st.Changed() || st.Ahead != 0 || st.Behind != 0 || st.Err != nil {
		t.Errorf("expected clean status, got %+v", st)
	}
	if st.Repo.Path != "acme" {
		t.Errorf("path = %q, want %q", st.Repo.Path, "acme")
	}
}

// TestStatusRepoStat: line diffs are probed only with --stat, and a probe
// failure (e.g. unborn HEAD) leaves the counts blank without failing the row.
func TestStatusRepoStat(t *testing.T) {
	t.Parallel()

	fake := fakeGit{
		workingDiff: git.DiffStat{Added: 27, Deleted: 8},
		snap:        git.Snapshot{Branch: "main", Tracking: true},
	}
	repo := domain.Repository{Name: "acme", AbsPath: "/tmp/acme", State: domain.RepoStateOK}
	project := domain.Project{Name: "p", Repos: []domain.Repository{repo}}

	st, _ := probe(t, statusDeps(t, fake), Probe{Stat: true}, project, repo)
	if st.Stat == nil || st.Stat.Added != 27 || st.Stat.Deleted != 8 {
		t.Errorf("line diffs = %+v, want +27 -8", st.Stat)
	}

	st, _ = probe(t, statusDeps(t, fake), Probe{}, project, repo)
	if st.Stat != nil {
		t.Errorf("line diffs probed without --stat: %+v", st.Stat)
	}

	fake.workingDiffErr = errors.New("unborn HEAD")
	st, err := probe(t, statusDeps(t, fake), Probe{Stat: true}, project, repo)
	if err != nil || st.Stat != nil {
		t.Errorf("diff probe failure should be tolerated, got err=%v %+v", err, st.Stat)
	}
}

// TestStatusRepoNoUpstream: a diff failure leaves the row uncompared instead
// of failing it.
func TestStatusRepoNoUpstream(t *testing.T) {
	t.Parallel()

	deps := statusDeps(t, fakeGit{
		snap:        git.Snapshot{Branch: "main"},
		fallbackRef: "origin/main",
		diffErr:     errors.New("unknown revision"),
	})
	repo := domain.Repository{Name: "acme", AbsPath: "/tmp/acme", State: domain.RepoStateOK}
	project := domain.Project{Name: "p", Repos: []domain.Repository{repo}}

	st, err := probe(t, deps, Probe{}, project, repo)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if st.Compared {
		t.Error("expected the row uncompared when the diff fails")
	}
}

// TestStatusRepoFallbackRef: with no upstream configured but a matching
// branch on a (possibly non-origin) remote, real ahead/behind counts show
// instead of N/A.
func TestStatusRepoFallbackRef(t *testing.T) {
	t.Parallel()

	deps := statusDeps(t, fakeGit{
		snap:        git.Snapshot{Branch: "main"},
		fallbackRef: "upstream/main",
		ahead:       2,
		behind:      1,
	})
	repo := domain.Repository{Name: "acme", AbsPath: "/tmp/acme", State: domain.RepoStateOK}
	project := domain.Project{Name: "p", Repos: []domain.Repository{repo}}

	st, err := probe(t, deps, Probe{}, project, repo)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !st.Compared || st.Ahead != 2 || st.Behind != 1 {
		t.Errorf("fallback divergence = %d/%d compared=%v, want 2/1 true",
			st.Ahead, st.Behind, st.Compared)
	}
}

// TestStatusRepoNoRemoteBranch: no upstream and no matching remote branch
// anywhere leaves the row N/A.
func TestStatusRepoNoRemoteBranch(t *testing.T) {
	t.Parallel()

	deps := statusDeps(t, fakeGit{snap: git.Snapshot{Branch: "main"}})
	repo := domain.Repository{Name: "acme", AbsPath: "/tmp/acme", State: domain.RepoStateOK}
	project := domain.Project{Name: "p", Repos: []domain.Repository{repo}}

	st, _ := probe(t, deps, Probe{}, project, repo)
	if st.Compared {
		t.Error("expected the row uncompared when no remote has the branch")
	}
}
