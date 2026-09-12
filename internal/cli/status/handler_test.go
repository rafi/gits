package status

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/bulk"
	"github.com/rafi/gits/internal/cli"
	"github.com/rafi/gits/internal/cli/clitest"
	"github.com/rafi/gits/internal/git"
	"github.com/rafi/gits/internal/types"
)

// The TestExecStatus tests drive ExecStatus — the command's real entry point —
// with explicit arguments, so argument parsing, the choice between walking a
// whole Project and probing a single Repository, both renderers and the error
// epilogue run for real, and the interactive finder is never reached. None of
// them builds a repoStatus or a collected result by hand: every assertion is
// made on the text a user would see and the error the command returns.

// execGit answers the status probe per repository, keyed by the directory name
// the Traversal hands it, so one project's repositories can differ from each
// other — which is what the filter tests turn on. A repository with no entry
// as clean and in sync with its upstream. Everything the probe does not call is
// inherited from clitest.FakeGit and panics if reached.
type execGit struct {
	clitest.FakeGit

	snaps   map[string]git.Snapshot
	failure error // fails the work-tree read of every repository
	// fallback is the ref every repository whose Upstream does not resolve is
	// compared against instead, with ahead/behind as that comparison's answer.
	// Empty — the usual fixture — means no Remote carries the branch.
	fallback      string
	ahead, behind int
}

func (g execGit) Snapshot(_ context.Context, path string) (git.Snapshot, error) {
	if g.failure != nil {
		return git.Snapshot{}, g.failure
	}
	if snap, ok := g.snaps[filepath.Base(path)]; ok {
		return snap, nil
	}
	return git.Snapshot{Branch: "main", Tracking: true}, nil
}

// FallbackRef answers with the fixture's ref, reached only for a snapshot whose
// Upstream does not resolve — none configured, or a Gone one.
func (g execGit) FallbackRef(context.Context, string, string) string { return g.fallback }

// Diff is the fallback comparison, reached only when FallbackRef named a ref.
func (g execGit) Diff(context.Context, string, string, string) (int, int, error) {
	return g.ahead, g.behind, nil
}

// HeadInfo names the repository in the commit subject, so an assertion can tell
// which repository's row it is reading, and carries a fixed describe string so
// the version column has something to render.
func (execGit) HeadInfo(_ context.Context, path string) (git.Head, error) {
	return git.Head{Hash: "abc1234", Subject: "Add " + filepath.Base(path), Describe: "v1.0.0"}, nil
}

// countingGit records every git method the status probe reaches, so a test can
// assert on the subprocess budget rather than trust the reading of the code.
// It answers the clean, tracking path: one Snapshot and one HeadInfo per
// repository, and nothing else.
type countingGit struct {
	clitest.FakeGit

	mu    sync.Mutex
	calls map[string]int
}

func (g *countingGit) record(name string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.calls == nil {
		g.calls = map[string]int{}
	}
	g.calls[name]++
}

func (g *countingGit) Snapshot(context.Context, string) (git.Snapshot, error) {
	g.record("Snapshot")
	return git.Snapshot{Branch: "main", Tracking: true}, nil
}

func (g *countingGit) HeadInfo(context.Context, string) (git.Head, error) {
	g.record("HeadInfo")
	return git.Head{Hash: "abc1234", Subject: "commit", Describe: "v1.0.0"}, nil
}

// total returns the number of git calls recorded across all methods.
func (g *countingGit) total() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	sum := 0
	for _, n := range g.calls {
		sum += n
	}
	return sum
}

// TestExecStatusSubprocessBudget pins the per-repository git cost: `status` over
// N clean repositories with tracking upstreams issues exactly 2N git calls —
// one Snapshot and one HeadInfo each — with no Describe, FallbackRef or Diff
// reached. The fake panics on any other method, so an unbudgeted call fails the
// test rather than passing quietly.
func TestExecStatusSubprocessBudget(t *testing.T) {
	t.Parallel()

	const n = 3
	g := &countingGit{}
	deps := clitest.New(t, g).
		WithProject("acme", clitest.Cloned("api"), clitest.Cloned("web"), clitest.Cloned("lib"))

	if err := ExecStatus("table", Options{}, []string{"acme"}, deps.RuntimeCLI); err != nil {
		t.Fatalf("ExecStatus error = %v, want nil", err)
	}

	if got := g.total(); got != 2*n {
		t.Errorf("git calls = %d (%v), want %d — one Snapshot and one HeadInfo per repository",
			got, g.calls, 2*n)
	}
	if g.calls["Snapshot"] != n || g.calls["HeadInfo"] != n {
		t.Errorf("call log = %v, want %d Snapshot and %d HeadInfo", g.calls, n, n)
	}
}

// TestExecStatusSplitsOutput is the piping guarantee: the status table is
// Result Output, the summary footer is Diagnostic Output, and neither leaks
// into the other. `gits status | jq` sees the table alone; the footer still
// reaches the terminal.
func TestExecStatusSplitsOutput(t *testing.T) {
	t.Parallel()

	deps := clitest.New(t, execGit{}).
		WithProject("acme", clitest.Cloned("api"), clitest.Cloned("web"))

	if err := ExecStatus("table", Options{}, []string{"acme"}, deps.RuntimeCLI); err != nil {
		t.Fatalf("ExecStatus error = %v, want nil", err)
	}

	result := deps.Result()
	for _, want := range []string{"Repo", "Branch", "Message",
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
	for _, banned := range []string{"api", "web", "Branch", "acme"} {
		if strings.Contains(diagnostic, banned) {
			t.Errorf("Diagnostic Output = %q, want no part of the table (%q)",
				diagnostic, banned)
		}
	}
}

// TestExecStatusSingleRepo covers `gits status acme api`: the second argument
// selects one repository, and only that one is probed and rendered as a
// one-row table.
func TestExecStatusSingleRepo(t *testing.T) {
	t.Parallel()

	deps := clitest.New(t, execGit{}).
		WithProject("acme", clitest.Cloned("api"), clitest.Cloned("web"))

	if err := ExecStatus("table", Options{}, []string{"acme", "api"}, deps.RuntimeCLI); err != nil {
		t.Fatalf("ExecStatus error = %v, want nil", err)
	}

	result := deps.Result()
	if !strings.Contains(result, "api") || !strings.Contains(result, "Add api") {
		t.Errorf("Result Output = %q, want the selected repository's row", result)
	}
	if strings.Contains(result, "web") {
		t.Errorf("Result Output = %q, want nothing about the other repository", result)
	}
	if got := deps.Diagnostic(); !strings.Contains(got, "○ Showing 1 repo") {
		t.Errorf("Diagnostic Output = %q, want a one-repository footer", got)
	}
}

// TestExecStatusSingleRepoState covers `gits status acme gone` on a repository
// that is not cloned: the row explains itself with the Reason the state was
// classified for, the epilogue names the repository, and the run fails.
func TestExecStatusSingleRepoState(t *testing.T) {
	t.Parallel()

	deps := clitest.New(t, execGit{}).
		WithProject("acme", clitest.Cloned("api"), clitest.NotCloned("gone"))

	err := ExecStatus("table", Options{}, []string{"acme", "gone"}, deps.RuntimeCLI)
	if err == nil {
		t.Fatal("ExecStatus error = nil, want the unusable repository to fail")
	}
	if got := deps.Diagnostic(); !strings.Contains(got, "1 error:") ||
		!strings.Contains(got, "gone") || !strings.Contains(got, "not cloned") {
		t.Errorf("Diagnostic Output = %q, want the epilogue naming the repository's condition", got)
	}
	if got := deps.Result(); !strings.Contains(got, "gone") ||
		!strings.Contains(got, "not cloned") {
		t.Errorf("Result Output = %q, want the row to explain itself", got)
	}
}

// TestExecStatusJSON covers `gits status -o json acme`: the envelope ADR-0001
// fixed, with the working-tree data nested under each repository git was asked
// about — and no such object on one it never saw. Diagnostic Output stays empty
// because the json form prints no footer at all: the whole document is Result
// Output, so it can be piped as one line.
func TestExecStatusJSON(t *testing.T) {
	t.Parallel()

	g := execGit{snaps: map[string]git.Snapshot{
		"api": {
			Branch:   "topic",
			Tracking: true,
			Ahead:    2,
			Behind:   1,
			WorkTree: git.WorkTree{Staged: 1, Unstaged: 2, Untracked: 3},
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

// defaultIcons is the icon set clitest wires into a command test, so an
// assertion names the glyph a user would see rather than repeating its literal.
func defaultIcons() domain.Icons {
	var icons domain.Icons
	icons.ApplyDefaults()
	return icons
}

// statusRow drives `gits status acme` over one repository and returns its
// Result Output, asserting the footer counted the row without an error — a
// Gone Upstream is a condition of the branch, not a failed probe.
func statusRow(t *testing.T, g execGit) string {
	t.Helper()
	deps := clitest.New(t, g).WithProject("acme", clitest.Cloned("api"))
	if err := ExecStatus("table", Options{}, []string{"acme"}, deps.RuntimeCLI); err != nil {
		t.Fatalf("ExecStatus error = %v, want nil", err)
	}
	if got := deps.Diagnostic(); !strings.Contains(got, "○ Showing 1 repo") ||
		strings.Contains(got, "error") {
		t.Errorf("Diagnostic Output = %q, want one repository counted and no error", got)
	}
	return deps.Result()
}

// TestExecStatusGoneUpstreamIcon covers the table telling the two conditions
// apart: a branch whose Upstream was merged and cleaned up gets its own glyph,
// where it used to share the not-applicable one with a branch that was never
// pushed.
func TestExecStatusGoneUpstreamIcon(t *testing.T) {
	t.Parallel()

	icons := defaultIcons()
	row := func(t *testing.T, snap git.Snapshot) string {
		t.Helper()
		return statusRow(t, execGit{snaps: map[string]git.Snapshot{"api": snap}})
	}

	gone := row(t, git.Snapshot{Branch: "feat-b", Upstream: "origin/feat-b"})
	none := row(t, git.Snapshot{Branch: "feat-b"})

	if !strings.Contains(gone, icons.Gone) {
		t.Errorf("Result Output = %q, want the Gone Upstream glyph %q", gone, icons.Gone)
	}
	if strings.Contains(gone, icons.NA) {
		t.Errorf("Result Output = %q, want the Gone Upstream not to share the %q glyph",
			gone, icons.NA)
	}
	if !strings.Contains(none, icons.NA) {
		t.Errorf("Result Output = %q, want a branch with no Upstream to keep %q",
			none, icons.NA)
	}
	if strings.Contains(none, icons.Gone) {
		t.Errorf("Result Output = %q, want no Gone Upstream glyph for a branch without one",
			none)
	}
}

// TestExecStatusGoneUpstreamRowKeepsCounts pins the deliberate ordering behind
// the new glyph: a Gone Upstream whose branch name still exists on a Remote is
// measured against it, and the row says both things — the glyph reports the
// Upstream's state, and the counts it was measured against stay legible in the
// Upstream⇅ column rather than being displaced by it.
func TestExecStatusGoneUpstreamRowKeepsCounts(t *testing.T) {
	t.Parallel()

	icons := defaultIcons()
	got := statusRow(t, execGit{
		snaps:    map[string]git.Snapshot{"api": {Branch: "feat-b", Upstream: "origin/feat-b"}},
		fallback: "upstream/feat-b",
		ahead:    2,
		behind:   1,
	})

	if !strings.Contains(got, icons.Gone) {
		t.Errorf("Result Output = %q, want the Gone Upstream glyph %q", got, icons.Gone)
	}
	for _, want := range []string{icons.Ahead + "2", icons.Behind + "1"} {
		if !strings.Contains(got, want) {
			t.Errorf("Result Output = %q, want the fallback comparison's %q", got, want)
		}
	}
}

// upstreamOf digs the nested upstream object out of one repository of a decoded
// status document, reporting whether it was there at all.
func upstreamOf(t *testing.T, doc map[string]any, project string, idx int) (map[string]any, bool) {
	t.Helper()
	upstream, found := docStatusOf(t, doc, project, idx)["upstream"].(map[string]any)
	return upstream, found
}

// docStatusOf digs one repository's status object out of a decoded document.
func docStatusOf(t *testing.T, doc map[string]any, project string, idx int) map[string]any {
	t.Helper()
	node, ok := doc[project].(map[string]any)
	if !ok {
		t.Fatalf("project %q missing from %v", project, doc)
	}
	repos, ok := node["repos"].([]any)
	if !ok || len(repos) <= idx {
		t.Fatalf("no repo %d in %v", idx, node)
	}
	status, ok := repos[idx].(map[string]any)["status"].(map[string]any)
	if !ok {
		t.Fatalf("no status object on repo %d: %v", idx, repos[idx])
	}
	return status
}

// statusDoc drives `gits status -o json` over one project and decodes it. The
// json form writes the whole document as Result Output and nothing else, so
// Diagnostic Output staying empty is asserted for every document built here.
func statusDoc(t *testing.T, g execGit, repos ...clitest.Repo) map[string]any {
	t.Helper()
	deps := clitest.New(t, g).WithProject("acme", repos...)
	if err := ExecStatus("json", Options{}, []string{"acme"}, deps.RuntimeCLI); err != nil {
		t.Fatalf("ExecStatus error = %v, want nil", err)
	}
	if got := deps.Diagnostic(); got != "" {
		t.Errorf("Diagnostic Output = %q, want the json form to emit none", got)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(deps.Result()), &doc); err != nil {
		t.Fatalf("unmarshal Result Output: %v", err)
	}
	return doc
}

// TestExecStatusJSONUpstream covers the envelope's addition in all three
// states: absent when no Upstream is configured, present and tracked for a
// healthy one, present and untracked for a Gone Upstream. The distinction is
// structural — no sentinel value carries it.
func TestExecStatusJSONUpstream(t *testing.T) {
	t.Parallel()

	g := execGit{snaps: map[string]git.Snapshot{
		"api":  {Branch: "main", Upstream: "origin/main", Tracking: true},
		"feat": {Branch: "feat-b", Upstream: "origin/feat-b"},
		"solo": {Branch: "wip"},
	}}
	doc := statusDoc(t, g,
		clitest.Cloned("api"), clitest.Cloned("feat"), clitest.Cloned("solo"))

	for _, tc := range []struct {
		name    string
		idx     int
		want    map[string]any
		present bool
	}{
		{name: "tracked", idx: 0, present: true,
			want: map[string]any{"name": "origin/main", "tracked": true}},
		{name: "gone", idx: 1, present: true,
			want: map[string]any{"name": "origin/feat-b", "tracked": false}},
		{name: "none", idx: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, found := upstreamOf(t, doc, "acme", tc.idx)
			if found != tc.present {
				t.Fatalf("upstream object present = %v, want %v: %v", found, tc.present, got)
			}
			for key, want := range tc.want {
				if got[key] != want {
					t.Errorf("upstream[%q] = %v, want %v", key, got[key], want)
				}
			}
		})
	}
}

// TestExecStatusJSONGoneUpstreamStillMeasured covers a Gone Upstream whose
// branch name still exists on a Remote: the comparison against that Remote is
// kept, so `compared` stays true and the counts are real. `compared` means a
// comparison was made — including against a fallback ref — and the upstream
// object beside it is what says the Upstream itself is gone.
func TestExecStatusJSONGoneUpstreamStillMeasured(t *testing.T) {
	t.Parallel()

	g := execGit{
		snaps:    map[string]git.Snapshot{"api": {Branch: "feat-b", Upstream: "origin/feat-b"}},
		fallback: "upstream/feat-b",
		ahead:    2,
		behind:   1,
	}
	doc := statusDoc(t, g, clitest.Cloned("api"))

	status := docStatusOf(t, doc, "acme", 0)
	if status["ahead"] != float64(2) || status["behind"] != float64(1) {
		t.Errorf("divergence = %v/%v, want the fallback comparison's 2/1",
			status["ahead"], status["behind"])
	}
	if status["compared"] != true {
		t.Errorf("compared = %v, want true — a comparison was made", status["compared"])
	}
	upstream, found := upstreamOf(t, doc, "acme", 0)
	if !found || upstream["tracked"] != false || upstream["name"] != "origin/feat-b" {
		t.Errorf("upstream = %v (present=%v), want the Gone Upstream named", upstream, found)
	}
}

// TestExecStatusUnknownProject covers naming a project that does not exist:
// it is a real failure, not a downgradeable warning, so a script can tell a
// typo from success. The error names the project and nothing is rendered.
func TestExecStatusUnknownProject(t *testing.T) {
	t.Parallel()

	for _, format := range []string{"table", "json"} {
		t.Run(format, func(t *testing.T) {
			t.Parallel()

			deps := clitest.New(t, execGit{}).
				WithProject("acme", clitest.Cloned("api"))

			err := ExecStatus(format, Options{}, []string{"typo"}, deps.RuntimeCLI)
			if err == nil {
				t.Fatalf("ExecStatus(%q, typo) = nil, want a real error", format)
			}
			if types.IsWarning(err) {
				t.Errorf("ExecStatus(%q, typo) error = %v, want a real error not a warning",
					format, err)
			}
			if !strings.Contains(err.Error(), "typo") {
				t.Errorf("ExecStatus(%q, typo) error = %v, want it to name the project", format, err)
			}
			if got := deps.Result(); got != "" {
				t.Errorf("Result Output = %q, want nothing rendered", got)
			}
		})
	}
}

// TestExecStatusUnknownFormat covers a format other than table or json being
// rejected before anything is loaded. The project's Provider Source is invalid,
// so a load would fail with its own error — the format error arriving instead is
// what says nothing was loaded.
func TestExecStatusUnknownFormat(t *testing.T) {
	t.Parallel()

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
	t.Parallel()

	// api has a staged change and is level with its upstream; web is clean and
	// one commit ahead; docs is neither, so no filter keeps it.
	g := execGit{snaps: map[string]git.Snapshot{
		"api": {Branch: "main", Tracking: true, WorkTree: git.WorkTree{Staged: 1}},
		"web": {Branch: "main", Tracking: true, Ahead: 1},
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
			t.Parallel()

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
	t.Parallel()

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
	t.Parallel()

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

// fakeGit stubs the status-relevant git.Reader methods. Status only reads, so
// the fake is a Reader plus refusing writes; even the clean path is
// exercisable here.
type fakeGit struct {
	git.Reader
	clitest.FakeNoWrites

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

// statusDeps builds the shared runtime dependencies for the tests that call
// statusRepo or renderTable directly — neither writes to a destination, so only
// the theme, the icons and the git client matter here.
func statusDeps(t *testing.T, g git.Client) types.RuntimeCLI {
	t.Helper()
	return clitest.New(t, g).RuntimeCLI
}

// probe drives statusRepo for one repository, assembling the bundled argument
// the module hands a body: the repository, its owning project and its display
// path.
func probe(
	t *testing.T,
	deps types.RuntimeCLI,
	opts Options,
	project domain.Project,
	repo domain.Repository,
) (*repoStatus, error) {
	t.Helper()
	return statusRepo(opts)(t.Context(), bulk.Repo{
		Repository: repo,
		Project:    project,
		Path:       cli.RepoRelPath(project, repo, deps.HomeDir),
	}, deps)
}

// TestStatusRepoProbeError: a failure reading the work tree surfaces as a
// counted error, carried on the row for its message cell.
func TestStatusRepoProbeError(t *testing.T) {
	t.Parallel()

	deps := statusDeps(t, fakeGit{snapErr: errors.New("boom")})
	repo := domain.Repository{Name: "acme", AbsPath: "/tmp/acme", State: domain.RepoStateOK}
	project := domain.Project{Name: "p", Repos: []domain.Repository{repo}}

	st, err := probe(t, deps, Options{}, project, repo)
	if err == nil {
		t.Fatal("expected an error when reading the work tree fails")
	}
	if st.err == nil || st.err.Error() != "boom" {
		t.Errorf("err = %v, want %q", st.err, "boom")
	}
	if cli.RenderErrors(io.Discard, []error{err}, true) == nil {
		t.Fatal("work-tree failure should count as a real error")
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

	st, err := probe(t, deps, Options{}, project, repo)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if st.Staged != 1 || st.Unstaged != 122 || st.Untracked != 4567 {
		t.Errorf("work tree = %d/%d/%d, want 1/122/4567",
			st.Staged, st.Unstaged, st.Untracked)
	}
	if st.Ahead != 3 || st.Behind != 1 || !st.compared {
		t.Errorf("divergence = %d/%d compared=%v, want 3/1 true",
			st.Ahead, st.Behind, st.compared)
	}
	if !st.changed() {
		t.Error("changed() = false for a dirty work tree")
	}
	if st.Branch != "main" || st.version != "v1.2.3" ||
		st.head.Hash != "abc12345" || st.head.Subject != "Add feature" {
		t.Errorf("metadata = %q %q %+v", st.Branch, st.version, st.head)
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

	st, err := probe(t, deps, Options{}, project, repo)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if st.changed() || st.Ahead != 0 || st.Behind != 0 || st.err != nil {
		t.Errorf("expected clean status, got %+v", st)
	}
	if st.repo.Path != "acme" {
		t.Errorf("path = %q, want %q", st.repo.Path, "acme")
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

	st, _ := probe(t, statusDeps(t, fake), Options{Stat: true}, project, repo)
	if st.stat == nil || st.stat.Added != 27 || st.stat.Deleted != 8 {
		t.Errorf("line diffs = %+v, want +27 -8", st.stat)
	}

	st, _ = probe(t, statusDeps(t, fake), Options{}, project, repo)
	if st.stat != nil {
		t.Errorf("line diffs probed without --stat: %+v", st.stat)
	}

	fake.workingDiffErr = errors.New("unborn HEAD")
	st, err := probe(t, statusDeps(t, fake), Options{Stat: true}, project, repo)
	if err != nil || st.stat != nil {
		t.Errorf("diff probe failure should be tolerated, got err=%v %+v", err, st.stat)
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

	st, err := probe(t, deps, Options{}, project, repo)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if st.compared {
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

	st, err := probe(t, deps, Options{}, project, repo)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !st.compared || st.Ahead != 2 || st.Behind != 1 {
		t.Errorf("fallback divergence = %d/%d compared=%v, want 2/1 true",
			st.Ahead, st.Behind, st.compared)
	}
}

// TestStatusRepoNoRemoteBranch: no upstream and no matching remote branch
// anywhere leaves the row N/A.
func TestStatusRepoNoRemoteBranch(t *testing.T) {
	t.Parallel()

	deps := statusDeps(t, fakeGit{snap: git.Snapshot{Branch: "main"}})
	repo := domain.Repository{Name: "acme", AbsPath: "/tmp/acme", State: domain.RepoStateOK}
	project := domain.Project{Name: "p", Repos: []domain.Repository{repo}}

	st, _ := probe(t, deps, Options{}, project, repo)
	if st.compared {
		t.Error("expected the row uncompared when no remote has the branch")
	}
}
