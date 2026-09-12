package status

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cli"
	"github.com/rafi/gits/internal/cli/clitest"
	"github.com/rafi/gits/internal/types"
	"github.com/rafi/gits/pkg/git"
)

// The TestExecStatus tests drive ExecStatus — the command's real entry point —
// with explicit arguments, so argument parsing, the choice between walking a
// whole Project and probing a single Repository, both renderers and the error
// epilogue run for real, and the interactive finder is never reached. None of
// them builds a repoStatus or a walk result by hand: every assertion is made on
// the text a user would see and the error the command returns.

// execGit answers the status probe per repository, keyed by the directory name
// the walker hands it, so one project's repositories can differ from each other
// — which is what the filter tests turn on. A repository with no entry answers
// as clean and in sync with its upstream. Everything the probe does not call is
// inherited from clitest.FakeGit and panics if reached.
type execGit struct {
	clitest.FakeGit
	snaps   map[string]git.Snapshot
	failure error // fails the work-tree read of every repository
}

func (g execGit) Snapshot(_ context.Context, path string) (git.Snapshot, error) {
	if g.failure != nil {
		return git.Snapshot{}, g.failure
	}
	if snap, ok := g.snaps[filepath.Base(path)]; ok {
		return snap, nil
	}
	return git.Snapshot{Branch: "main", HasUpstream: true}, nil
}

func (execGit) Describe(context.Context, string) (string, error) { return "v1.0.0", nil }

// FallbackRef finds no comparable branch on any Remote, which is only reached
// for a snapshot without an upstream — so no fixture needs Diff.
func (execGit) FallbackRef(context.Context, string, string) string { return "" }

// HeadInfo names the repository in the commit subject, so an assertion can tell
// which repository's row it is reading.
func (execGit) HeadInfo(_ context.Context, path string) (git.Head, error) {
	return git.Head{Hash: "abc1234", Subject: "Add " + filepath.Base(path)}, nil
}

// TestExecStatusSplitsOutput is the piping guarantee: the status table is
// Result Output, the summary footer is Diagnostic Output, and neither leaks
// into the other. `gits status | jq` sees the table alone; the footer still
// reaches the terminal.
func TestExecStatusSplitsOutput(t *testing.T) {
	deps := clitest.New(t, execGit{}).
		WithProject("acme", clitest.Cloned("api"), clitest.Cloned("web"))

	if err := ExecStatus("table", Options{}, []string{"acme"}, deps.RuntimeCLI); err != nil {
		t.Fatalf("ExecStatus error = %v, want nil", err)
	}

	result := deps.Result()
	for _, want := range []string{":: acme", "Repo", "Branch", "Message",
		"api", "web", "Add api", "main"} {
		if !strings.Contains(result, want) {
			t.Errorf("Result Output = %q, want it to contain %q", result, want)
		}
	}
	if strings.Contains(result, "○ Showing") {
		t.Errorf("Result Output = %q, want the summary footer kept out of it", result)
	}

	diagnostic := deps.Diagnostic()
	if !strings.Contains(diagnostic, "○ Showing 2 repos") {
		t.Errorf("Diagnostic Output = %q, want the summary footer", diagnostic)
	}
	for _, banned := range []string{"api", "web", "Branch", ":: acme"} {
		if strings.Contains(diagnostic, banned) {
			t.Errorf("Diagnostic Output = %q, want no part of the table (%q)",
				diagnostic, banned)
		}
	}
}

// TestExecStatusSingleRepo covers `gits status acme api`: the second argument
// selects one repository, and only that one is probed and rendered — as a
// one-row table without the project title the whole-project path prints.
func TestExecStatusSingleRepo(t *testing.T) {
	deps := clitest.New(t, execGit{}).
		WithProject("acme", clitest.Cloned("api"), clitest.Cloned("web"))

	if err := ExecStatus("table", Options{}, []string{"acme", "api"}, deps.RuntimeCLI); err != nil {
		t.Fatalf("ExecStatus error = %v, want nil", err)
	}

	result := deps.Result()
	if !strings.Contains(result, "api") || !strings.Contains(result, "Add api") {
		t.Errorf("Result Output = %q, want the selected repository's row", result)
	}
	for _, banned := range []string{"web", ":: acme"} {
		if strings.Contains(result, banned) {
			t.Errorf("Result Output = %q, want nothing about %q", result, banned)
		}
	}
	if got := deps.Diagnostic(); !strings.Contains(got, "○ Showing 1 repo") {
		t.Errorf("Diagnostic Output = %q, want a one-repository footer", got)
	}
}

// TestExecStatusJSON covers `gits status -o json acme`: the envelope ADR-0001
// fixed, with the working-tree data nested under each repository git was asked
// about — and no such object on one it never saw. Diagnostic Output stays empty
// because the json form prints no footer at all: the whole document is Result
// Output, so it can be piped as one line.
func TestExecStatusJSON(t *testing.T) {
	g := execGit{snaps: map[string]git.Snapshot{
		"api": {
			Branch:      "topic",
			HasUpstream: true,
			Ahead:       2,
			Behind:      1,
			WorkTree:    git.WorkTree{Staged: 1, Unstaged: 2, Untracked: 3},
		},
	}}
	deps := clitest.New(t, g).
		WithProject("acme", clitest.Cloned("api"), clitest.NotCloned("gone"))

	if err := ExecStatus("json", Options{}, []string{"acme"}, deps.RuntimeCLI); err != nil {
		t.Fatalf("ExecStatus error = %v, want nil — a repository's condition is data", err)
	}

	raw := deps.Result()
	if got := strings.Count(raw, "\n"); got != 1 || !strings.HasSuffix(raw, "\n") {
		t.Errorf("Result Output = %q, want one newline-terminated line", raw)
	}
	if got := deps.Diagnostic(); got != "" {
		t.Errorf("Diagnostic Output = %q, want empty", got)
	}

	// Decoded structurally rather than through the envelope types, so the wire
	// contract is checked against something other than itself.
	var env map[string]struct {
		Repos []struct {
			Name   string `json:"name"`
			State  string `json:"state"`
			Status *struct {
				Branch    string `json:"branch"`
				Staged    int    `json:"staged"`
				Unstaged  int    `json:"unstaged"`
				Untracked int    `json:"untracked"`
				Ahead     int    `json:"ahead"`
				Behind    int    `json:"behind"`
				Compared  bool   `json:"compared"`
				Version   string `json:"version"`
				Commit    *struct {
					Hash    string `json:"hash"`
					Subject string `json:"subject"`
				} `json:"commit"`
			} `json:"status"`
		} `json:"repos"`
	}
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		t.Fatalf("unmarshal Result Output: %v", err)
	}

	proj, ok := env["acme"]
	if !ok {
		t.Fatalf("envelope = %+v, want the project keyed by its name", env)
	}
	if len(proj.Repos) != 2 {
		t.Fatalf("repos = %+v, want both repositories", proj.Repos)
	}

	api := proj.Repos[0]
	if api.Name != "api" || api.State != string(domain.RepoStateOK) {
		t.Errorf("identity/state = %q/%q, want api/ok", api.Name, api.State)
	}
	if api.Status == nil {
		t.Fatalf("api = %+v, want the working tree nested under it", api)
	}
	got := *api.Status
	if got.Branch != "topic" || got.Version != "v1.0.0" {
		t.Errorf("branch/version = %q/%q, want topic/v1.0.0", got.Branch, got.Version)
	}
	if got.Staged != 1 || got.Unstaged != 2 || got.Untracked != 3 {
		t.Errorf("work tree = %d/%d/%d, want 1/2/3", got.Staged, got.Unstaged, got.Untracked)
	}
	if got.Ahead != 2 || got.Behind != 1 || !got.Compared {
		t.Errorf("divergence = %d/%d compared=%v, want 2/1 true",
			got.Ahead, got.Behind, got.Compared)
	}
	if got.Commit == nil || got.Commit.Hash != "abc1234" || got.Commit.Subject != "Add api" {
		t.Errorf("commit = %+v, want the last commit git reported", got.Commit)
	}

	// The repository git never saw carries its state and nothing more: the
	// key's absence is what says it was not probed.
	gone := proj.Repos[1]
	if gone.State != string(domain.RepoStateNotCloned) {
		t.Errorf("gone state = %q, want %q", gone.State, domain.RepoStateNotCloned)
	}
	if gone.Status != nil {
		t.Errorf("gone status = %+v, want none for a repository git never saw", gone.Status)
	}
}

// TestExecStatusUnknownFormat covers a format other than table or json being
// rejected before anything is loaded. The project's Provider Source is invalid,
// so a load would fail with its own error — the format error arriving instead is
// what says nothing was loaded.
func TestExecStatusUnknownFormat(t *testing.T) {
	deps := clitest.New(t, execGit{})
	deps.Projects = domain.ProjectListKeyed{"acme": {
		Source: &domain.ProviderSource{Type: "nope"},
		Repos:  []domain.Repository{{Name: "api"}},
	}}

	err := ExecStatus("wide", Options{}, []string{"acme"}, deps.RuntimeCLI)
	if err == nil {
		t.Fatal("ExecStatus(\"wide\") error = nil, want a rejected format")
	}
	if !strings.Contains(err.Error(), "unknown output format") {
		t.Errorf("error = %v, want it to name the unknown output format", err)
	}
	if got := deps.Result(); got != "" {
		t.Errorf("Result Output = %q, want nothing rendered", got)
	}
	if got := deps.Diagnostic(); got != "" {
		t.Errorf("Diagnostic Output = %q, want nothing rendered", got)
	}

	// The same dependencies with an accepted format do reach the loader and
	// fail there, which is what makes the assertion above meaningful.
	if err := ExecStatus("table", Options{}, []string{"acme"}, deps.RuntimeCLI); err == nil ||
		strings.Contains(err.Error(), "unknown output format") {
		t.Errorf("ExecStatus(\"table\") error = %v, want the load to have been attempted", err)
	}
}

// TestExecStatusFilters covers --dirty and --unsynced reaching both halves of
// the output: the rows the table renders, and the count of the ones it hid,
// which only the footer reports.
func TestExecStatusFilters(t *testing.T) {
	// api has a staged change and is level with its upstream; web is clean and
	// one commit ahead; docs is neither, so no filter keeps it.
	g := execGit{snaps: map[string]git.Snapshot{
		"api": {Branch: "main", HasUpstream: true, WorkTree: git.WorkTree{Staged: 1}},
		"web": {Branch: "main", HasUpstream: true, Ahead: 1},
	}}

	for _, tc := range []struct {
		name   string
		opts   Options
		shown  string
		hidden []string
		footer []string
	}{
		{
			name: "dirty", opts: Options{Dirty: true},
			shown: "api", hidden: []string{"web", "docs"},
			footer: []string{"○ Showing 1 repo", "1 with changes", "2 hidden"},
		},
		{
			name: "unsynced", opts: Options{Unsynced: true},
			shown: "web", hidden: []string{"api", "docs"},
			footer: []string{"○ Showing 1 repo", "1 ahead", "2 hidden"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			deps := clitest.New(t, g).WithProject("acme",
				clitest.Cloned("api"), clitest.Cloned("web"), clitest.Cloned("docs"))

			if err := ExecStatus("table", tc.opts, []string{"acme"}, deps.RuntimeCLI); err != nil {
				t.Fatalf("ExecStatus error = %v, want nil", err)
			}

			result := deps.Result()
			if !strings.Contains(result, tc.shown) {
				t.Errorf("Result Output = %q, want the %s repository %q",
					result, tc.name, tc.shown)
			}
			for _, banned := range tc.hidden {
				if strings.Contains(result, banned) {
					t.Errorf("Result Output = %q, want %q filtered out", result, banned)
				}
			}
			diagnostic := deps.Diagnostic()
			for _, want := range tc.footer {
				if !strings.Contains(diagnostic, want) {
					t.Errorf("Diagnostic Output = %q, want the footer to contain %q",
						diagnostic, want)
				}
			}
		})
	}
}

// TestExecStatusRepoState covers a repository in a non-ok Repo State reaching
// the user with the Reason it was classified for, rather than a generic
// message, and counting toward the exit code.
func TestExecStatusRepoState(t *testing.T) {
	deps := clitest.New(t, execGit{}).WithProject("acme",
		clitest.Cloned("api"), clitest.NotCloned("gone"), clitest.Broken("bad"))

	err := ExecStatus("table", Options{}, []string{"acme"}, deps.RuntimeCLI)
	if err == nil {
		t.Fatal("ExecStatus error = nil, want the unusable repositories to fail the run")
	}

	result := deps.Result()
	for _, want := range []string{"not cloned", clitest.BrokenReason} {
		if !strings.Contains(result, want) {
			t.Errorf("Result Output = %q, want the row to explain itself with %q", result, want)
		}
	}

	diagnostic := deps.Diagnostic()
	if !strings.Contains(diagnostic, "○ Showing 3 repos, 2 errors") {
		t.Errorf("Diagnostic Output = %q, want both failures counted in the footer", diagnostic)
	}
	if !strings.Contains(diagnostic, "2 errors:") ||
		!strings.Contains(diagnostic, clitest.BrokenReason) {
		t.Errorf("Diagnostic Output = %q, want the error epilogue", diagnostic)
	}
}

// TestExecStatusProbeFailure covers the work-tree read failing: the reason
// lands in the repository's row on Result Output and in the epilogue on
// Diagnostic Output, and the run reports failure.
func TestExecStatusProbeFailure(t *testing.T) {
	deps := clitest.New(t, execGit{failure: errors.New("boom")}).
		WithProject("acme", clitest.Cloned("api"))

	err := ExecStatus("table", Options{}, []string{"acme"}, deps.RuntimeCLI)
	if err == nil {
		t.Fatal("ExecStatus error = nil, want the failed probe to fail the run")
	}
	if !strings.Contains(err.Error(), "completed with errors") {
		t.Errorf("ExecStatus error = %v, want the run reported as failed", err)
	}
	if got := deps.Result(); !strings.Contains(got, "boom") {
		t.Errorf("Result Output = %q, want the failure on the repository's row", got)
	}
	if got := deps.Diagnostic(); !strings.Contains(got, "1 error:") ||
		!strings.Contains(got, "boom") {
		t.Errorf("Diagnostic Output = %q, want the error epilogue", got)
	}
}

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

// statusDeps builds the shared runtime dependencies for the tests that call
// statusRepo or renderTable directly — neither writes to a destination, so only
// the theme, the icons and the git client matter here.
func statusDeps(t *testing.T, g git.GitClient) types.RuntimeCLI {
	t.Helper()
	return clitest.New(t, g).RuntimeCLI
}

// TestStatusRepoNotCloned: a non-OK repo is reported via the output-free
// state-error path and counts as a failure.
func TestStatusRepoNotCloned(t *testing.T) {
	deps := statusDeps(t, fakeGit{})
	repo := domain.Repository{Name: "acme", AbsPath: "/tmp/acme", State: domain.RepoStateNotCloned}
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
	if cli.RenderErrors(io.Discard, []error{res.Err}, true) == nil {
		t.Fatal("non-cloned repo should count as a failure")
	}
}

// TestStatusRepoProbeError: a failure reading the work tree surfaces as a
// counted error with the reason in the row message.
func TestStatusRepoProbeError(t *testing.T) {
	deps := statusDeps(t, fakeGit{snapErr: errors.New("boom")})
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
	if cli.RenderErrors(io.Discard, []error{res.Err}, true) == nil {
		t.Fatal("work-tree failure should count as a real error")
	}
}

// TestStatusRepoDirty: work-tree counts and upstream divergence land in the
// structured payload.
func TestStatusRepoDirty(t *testing.T) {
	when := time.Now().Add(-2 * time.Hour)
	deps := statusDeps(t, fakeGit{
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
	deps := statusDeps(t, fakeGit{
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

	res := statusRepo(Options{Stat: true})(context.Background(), project, repo, statusDeps(t, fake))
	st := res.Payload.(*repoStatus)
	if st.added != 27 || st.deleted != 8 {
		t.Errorf("line diffs = +%d -%d, want +27 -8", st.added, st.deleted)
	}

	res = statusRepo(Options{})(context.Background(), project, repo, statusDeps(t, fake))
	st = res.Payload.(*repoStatus)
	if st.added != 0 || st.deleted != 0 {
		t.Errorf("line diffs probed without --stat: +%d -%d", st.added, st.deleted)
	}

	fake.workingDiffErr = errors.New("unborn HEAD")
	res = statusRepo(Options{Stat: true})(context.Background(), project, repo, statusDeps(t, fake))
	st = res.Payload.(*repoStatus)
	if res.Err != nil || st.added != 0 || st.deleted != 0 {
		t.Errorf("diff probe failure should be tolerated, got err=%v +%d -%d",
			res.Err, st.added, st.deleted)
	}
}

// TestStatusRepoNoUpstream: a diff failure flags noUpstream instead of failing
// the row.
func TestStatusRepoNoUpstream(t *testing.T) {
	deps := statusDeps(t, fakeGit{
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
	deps := statusDeps(t, fakeGit{
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
	deps := statusDeps(t, fakeGit{snap: git.Snapshot{Branch: "main"}})
	repo := domain.Repository{Name: "acme", AbsPath: "/tmp/acme", State: domain.RepoStateOK}
	project := domain.Project{Name: "p", Repos: []domain.Repository{repo}}

	res := statusRepo(Options{})(context.Background(), project, repo, deps)
	st := res.Payload.(*repoStatus)
	if !st.noUpstream {
		t.Error("expected noUpstream when no remote has the branch")
	}
}
