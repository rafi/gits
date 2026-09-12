package walk

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cli/clitest"
	"github.com/rafi/gits/internal/types"
)

// testDeps builds test dependencies with the given worker count, enough to
// drive Walk without touching a real git client. Both output destinations are
// buffers, so the walker's Result Output and Diagnostic Output are captured
// separately and the progress reporter falls silent.
func testDeps(t *testing.T, workers int) *clitest.Deps {
	t.Helper()
	deps := clitest.New(t, nil)
	deps.Settings.WorkerCount = workers
	return deps
}

// repo is a tiny helper to declare a repo by name.
func repo(name string) domain.Repository {
	return domain.Repository{Name: name, State: domain.RepoStateOK}
}

// lineFor returns a stable, greppable result line for a repo.
func lineFor(r domain.Repository) string { return "LINE:" + r.Name }

// echoFunc is a RepoFunc that just echoes the repo name, optionally sleeping.
func echoFunc(delay func(domain.Repository) time.Duration) RepoFunc {
	return func(ctx context.Context, _ domain.Project, r domain.Repository, _ types.RuntimeCLI) RepoResult {
		if delay != nil {
			if d := delay(r); d > 0 {
				select {
				case <-time.After(d):
				case <-ctx.Done():
				}
			}
		}
		return RepoResult{Line: lineFor(r)}
	}
}

// TestWalkStableOrder verifies AC-5: results render in tree order regardless of
// completion order. Repos complete in reverse order (later names finish first)
// but must still print alphabetically under the project header.
func TestWalkStableOrder(t *testing.T) {
	project := domain.Project{
		Name:  "proj",
		Repos: []domain.Repository{repo("alpha"), repo("bravo"), repo("charlie"), repo("delta")},
	}
	// Reverse completion: "alpha" sleeps longest, "delta" returns first.
	order := map[string]time.Duration{
		"alpha":   80 * time.Millisecond,
		"bravo":   60 * time.Millisecond,
		"charlie": 40 * time.Millisecond,
		"delta":   20 * time.Millisecond,
	}
	fn := echoFunc(func(r domain.Repository) time.Duration {
		return order[r.Name]
	})

	deps := testDeps(t, 4)
	errs := Walk(context.Background(), project, deps.RuntimeCLI, "testing", fn)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}

	got := deps.Result()
	want := []string{lineFor(repo("alpha")), lineFor(repo("bravo")), lineFor(repo("charlie")), lineFor(repo("delta"))}
	last := -1
	for _, w := range want {
		i := strings.Index(got, w)
		if i < 0 {
			t.Fatalf("missing %q in output:\n%s", w, got)
		}
		if i < last {
			t.Fatalf("out of order: %q appears before a prior repo:\n%s", w, got)
		}
		last = i
	}
}

// TestWalkSubProjectOrder verifies AC-5 across the tree: each project prints its
// own header once, repos first, then sub-projects depth-first.
func TestWalkSubProjectOrder(t *testing.T) {
	project := domain.Project{
		Name:  "root",
		Repos: []domain.Repository{repo("r1"), repo("r2")},
		SubProjects: []domain.Project{
			{Name: "sub", Repos: []domain.Repository{repo("s1")}},
		},
	}
	deps := testDeps(t, 2)
	Walk(context.Background(), project, deps.RuntimeCLI, "testing", echoFunc(nil))

	got := deps.Result()
	for _, seq := range []string{"root", lineFor(repo("r1")), lineFor(repo("r2")), "sub", lineFor(repo("s1"))} {
		if !strings.Contains(got, seq) {
			t.Fatalf("missing %q in output:\n%s", seq, got)
		}
	}
	// root header before sub header; r2 before s1.
	if strings.Index(got, "root") > strings.Index(got, "sub") {
		t.Fatalf("root header should precede sub header:\n%s", got)
	}
	if strings.Index(got, lineFor(repo("r2"))) > strings.Index(got, lineFor(repo("s1"))) {
		t.Fatalf("root repos should precede sub repos:\n%s", got)
	}
}

// TestWalkUsesDependencyDestinations proves the walker writes where its
// dependencies say: every rendered line is Result Output, and a non-terminal
// Diagnostic Output yields the no-op reporter, so no progress or ANSI cursor
// control can reach a captured run's Result Output.
func TestWalkUsesDependencyDestinations(t *testing.T) {
	project := domain.Project{Name: "proj", Repos: []domain.Repository{repo("a"), repo("b")}}

	deps := testDeps(t, 2)
	Walk(context.Background(), project, deps.RuntimeCLI, "testing", echoFunc(nil))

	for _, want := range []string{lineFor(repo("a")), lineFor(repo("b"))} {
		if !strings.Contains(deps.Result(), want) {
			t.Fatalf("Result Output missing %q:\n%s", want, deps.Result())
		}
	}
	if diag := deps.Diagnostic(); diag != "" {
		t.Fatalf("Diagnostic Output should be silent without a terminal, got:\n%q", diag)
	}

	single := testDeps(t, 1)
	if err := Single(context.Background(), project, repo("solo"),
		single.RuntimeCLI, echoFunc(nil)); err != nil {
		t.Fatalf("Single: %v", err)
	}
	if !strings.Contains(single.Result(), lineFor(repo("solo"))) {
		t.Fatalf("Single wrote no Result Output:\n%s", single.Result())
	}
	if diag := single.Diagnostic(); diag != "" {
		t.Fatalf("Single wrote Diagnostic Output it should not, got:\n%q", diag)
	}
}

// TestWalkNoStall verifies AC-1: a free worker immediately picks the next task,
// so total wall-clock is bounded well under the fully serial sum.
func TestWalkNoStall(t *testing.T) {
	repos := []domain.Repository{repo("slow")}
	for i := range 19 {
		repos = append(repos, repo(fmt.Sprintf("fast%02d", i)))
	}
	project := domain.Project{Name: "proj", Repos: repos}
	fn := echoFunc(func(r domain.Repository) time.Duration {
		if r.Name == "slow" {
			return 200 * time.Millisecond
		}
		return 40 * time.Millisecond
	})
	// Serial sum = 200 + 19*40 = 960ms; pool of 4 should finish well under 600ms.
	start := time.Now()
	Walk(context.Background(), project, testDeps(t, 4).RuntimeCLI, "testing", fn)
	if elapsed := time.Since(start); elapsed > 600*time.Millisecond {
		t.Fatalf("walk stalled: took %s, expected well under serial 960ms", elapsed)
	}
}

// TestWalkWorkerClamp verifies AC-2: WorkerCount <= 0 runs with one worker and
// never panics on a modulo or zero-sized pool.
func TestWalkWorkerClamp(t *testing.T) {
	project := domain.Project{Name: "proj", Repos: []domain.Repository{repo("a"), repo("b")}}
	deps := testDeps(t, 0)
	errs := Walk(context.Background(), project, deps.RuntimeCLI, "testing", echoFunc(nil))
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if !strings.Contains(deps.Result(), lineFor(repo("a"))) || !strings.Contains(deps.Result(), lineFor(repo("b"))) {
		t.Fatalf("clamped walk did not process all repos:\n%s", deps.Result())
	}
}

// TestWalkErrorAggregation verifies AC-10: warnings are excluded from the
// failing-error count while real errors are reported.
func TestWalkErrorAggregation(t *testing.T) {
	project := domain.Project{
		Name:  "proj",
		Repos: []domain.Repository{repo("ok"), repo("warn"), repo("boom")},
	}
	fn := func(_ context.Context, _ domain.Project, r domain.Repository, _ types.RuntimeCLI) RepoResult {
		switch r.Name {
		case "warn":
			return RepoResult{Line: lineFor(r), Err: types.NewWarning("just a warning")}
		case "boom":
			return RepoResult{Line: lineFor(r), Err: fmt.Errorf("real failure")}
		default:
			return RepoResult{Line: lineFor(r)}
		}
	}
	errs := Walk(context.Background(), project, testDeps(t, 2).RuntimeCLI, "testing", fn)
	if len(errs) != 2 {
		t.Fatalf("expected 2 aggregated errors (warning + error), got %d: %v", len(errs), errs)
	}
	// The walker returns all; RenderErrors(_, true) must keep only the real error.
	failing := 0
	for _, e := range errs {
		if types.IsWarning(e) {
			continue
		}
		failing++
	}
	if failing != 1 {
		t.Fatalf("expected exactly 1 non-warning failure, got %d", failing)
	}
}

// TestWalkCancellation verifies AC-9: cancelling mid-run leaves queued tasks
// unstarted (started < total) and returns promptly.
func TestWalkCancellation(t *testing.T) {
	const total = 40
	repos := make([]domain.Repository, total)
	for i := range repos {
		repos[i] = repo(fmt.Sprintf("r%03d", i))
	}
	project := domain.Project{Name: "proj", Repos: repos}

	var started atomic.Int64
	ctx, cancel := context.WithCancel(context.Background())
	fn := func(c context.Context, _ domain.Project, r domain.Repository, _ types.RuntimeCLI) RepoResult {
		n := started.Add(1)
		if n == 4 {
			cancel() // trip cancellation a few tasks in
		}
		select {
		case <-time.After(50 * time.Millisecond):
		case <-c.Done():
		}
		return RepoResult{Line: lineFor(r)}
	}

	deps := testDeps(t, 2)
	done := make(chan []error, 1)
	go func() {
		done <- Walk(ctx, project, deps.RuntimeCLI, "testing", fn)
	}()
	var errs []error
	select {
	case errs = <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("walk did not return promptly after cancellation")
	}
	if s := started.Load(); s >= total {
		t.Fatalf("cancellation did not stop queued tasks: started=%d total=%d", s, total)
	}

	// An interrupted run must fail loudly: exactly one non-warning error
	// naming how many repos were skipped, so the process exits non-zero
	// instead of silently reporting partial success.
	var interrupted error
	for _, e := range errs {
		if strings.Contains(e.Error(), "interrupted") {
			interrupted = e
		}
	}
	if interrupted == nil {
		t.Fatalf("no interruption error surfaced after cancellation: %v", errs)
	}
	if !isFailure(interrupted) {
		t.Fatalf("interruption error %v must not be a warning", interrupted)
	}
}

// TestCollectCancellationError verifies the Collect path also surfaces the
// interruption: callers rendering grouped results must learn the run was cut
// short.
func TestCollectCancellationError(t *testing.T) {
	const total = 40
	repos := make([]domain.Repository, total)
	for i := range repos {
		repos[i] = repo(fmt.Sprintf("r%03d", i))
	}
	project := domain.Project{Name: "proj", Repos: repos}

	ctx, cancel := context.WithCancel(context.Background())
	var started atomic.Int64
	fn := func(c context.Context, _ domain.Project, r domain.Repository, _ types.RuntimeCLI) RepoResult {
		if started.Add(1) == 4 {
			cancel()
		}
		select {
		case <-time.After(50 * time.Millisecond):
		case <-c.Done():
		}
		return RepoResult{Line: lineFor(r)}
	}

	deps := testDeps(t, 2)
	groups, err := collectReport(ctx, project, deps.RuntimeCLI, "testing", fn, &nopReporter{})
	if err == nil {
		t.Fatal("Collect after cancellation returned nil error, want interruption")
	}
	if !strings.Contains(err.Error(), "interrupted") {
		t.Fatalf("interruption error = %v, want mention of interruption", err)
	}
	if len(groups) == 0 {
		t.Fatal("cancelled Collect should still return partial groups")
	}

	// A clean run must stay error-free.
	groups, err = collectReport(context.Background(), project, deps.RuntimeCLI, "testing",
		echoFunc(nil), &nopReporter{})
	if err != nil {
		t.Fatalf("uninterrupted Collect returned %v, want nil", err)
	}
	if len(groups) != 1 || len(groups[0].Results) != total {
		t.Fatalf("unexpected groups shape: %d", len(groups))
	}
}

// fakeReporter records the per-repo tracker lifecycle so the walker's wiring can
// be asserted without a TTY. All counters are guarded for concurrent workers.
type fakeReporter struct {
	mu                  sync.Mutex
	repoStarts, done    int
	marksDone, marksErr int
}

func (r *fakeReporter) Start(string, int) {}
func (r *fakeReporter) RepoStart(string) RepoTracker {
	r.mu.Lock()
	r.repoStarts++
	r.mu.Unlock()
	return &fakeRepoTracker{r: r}
}
func (r *fakeReporter) Done() {
	r.mu.Lock()
	r.done++
	r.mu.Unlock()
}
func (r *fakeReporter) Stop() {}

type fakeRepoTracker struct{ r *fakeReporter }

func (t *fakeRepoTracker) MarkDone() {
	t.r.mu.Lock()
	t.r.marksDone++
	t.r.mu.Unlock()
}
func (t *fakeRepoTracker) MarkErrored() {
	t.r.mu.Lock()
	t.r.marksErr++
	t.r.mu.Unlock()
}

// TestLiveReporterErrorCount verifies the failed-count is owned by the
// reporter's trackers: N concurrent MarkErrored calls settle on exactly N,
// double completion marks count once, and MarkDone contributes nothing.
func TestLiveReporterErrorCount(t *testing.T) {
	const failures = 8
	r := newLiveReporter(&bytes.Buffer{})
	r.Start("testing", failures+1)

	var wg sync.WaitGroup
	for i := range failures {
		tracker := r.RepoStart(fmt.Sprintf("boom%02d", i))
		wg.Go(func() {
			tracker.MarkErrored()
			tracker.MarkErrored() // double completion must count once
			r.Done()
		})
	}
	okTracker := r.RepoStart("ok")
	wg.Go(func() {
		okTracker.MarkDone()
		r.Done()
	})
	wg.Wait()
	r.Stop()

	r.mu.Lock()
	got := r.errs
	r.mu.Unlock()
	if got != failures {
		t.Fatalf("reporter errs = %d, want %d", got, failures)
	}
}

// TestWalkErrorCount verifies the walker end-to-end: N failing repos leave
// the live reporter's error count at exactly N with no walker-side counting.
func TestWalkErrorCount(t *testing.T) {
	const failures = 8
	repos := make([]domain.Repository, failures)
	for i := range repos {
		repos[i] = repo(fmt.Sprintf("boom%02d", i))
	}
	project := domain.Project{Name: "proj", Repos: repos}
	fn := func(_ context.Context, _ domain.Project, r domain.Repository, _ types.RuntimeCLI) RepoResult {
		return RepoResult{Line: lineFor(r), Err: fmt.Errorf("real failure")}
	}
	rep := newLiveReporter(&bytes.Buffer{})
	// One worker per repo: all failures race to mark errored at once.
	errs := walkReport(context.Background(), project, testDeps(t, failures).RuntimeCLI, "testing", fn, rep)
	if len(errs) != failures {
		t.Fatalf("aggregated errors = %d, want %d", len(errs), failures)
	}
	rep.mu.Lock()
	got := rep.errs
	rep.mu.Unlock()
	if got != failures {
		t.Fatalf("reporter errs = %d, want %d", got, failures)
	}
}

// TestWalkPerRepoTrackers verifies the walker starts one tracker per repo and
// marks each done or errored by outcome: a real failure is errored, a warning
// and a success are done; the failed-count is reported to the overall tracker.
func TestWalkPerRepoTrackers(t *testing.T) {
	project := domain.Project{
		Name:  "proj",
		Repos: []domain.Repository{repo("ok"), repo("warn"), repo("boom")},
	}
	fn := func(_ context.Context, _ domain.Project, r domain.Repository, _ types.RuntimeCLI) RepoResult {
		switch r.Name {
		case "warn":
			return RepoResult{Line: lineFor(r), Err: types.NewWarning("just a warning")}
		case "boom":
			return RepoResult{Line: lineFor(r), Err: fmt.Errorf("real failure")}
		default:
			return RepoResult{Line: lineFor(r)}
		}
	}
	rep := &fakeReporter{}
	walkReport(context.Background(), project, testDeps(t, 2).RuntimeCLI, "testing", fn, rep)

	if rep.repoStarts != 3 {
		t.Errorf("repoStarts = %d, want 3", rep.repoStarts)
	}
	if rep.done != 3 {
		t.Errorf("done = %d, want 3", rep.done)
	}
	if rep.marksErr != 1 {
		t.Errorf("marksErr = %d, want 1 (only the real failure)", rep.marksErr)
	}
	if rep.marksDone != 2 {
		t.Errorf("marksDone = %d, want 2 (success + warning)", rep.marksDone)
	}
}

// TestCollectGroupOrder verifies Collect returns results grouped per project in
// stable tree order, with payloads intact and no line rendering.
func TestCollectGroupOrder(t *testing.T) {
	project := domain.Project{
		Name:  "root",
		Repos: []domain.Repository{repo("r1"), repo("r2")},
		SubProjects: []domain.Project{
			{Name: "sub", Repos: []domain.Repository{repo("s1")}},
		},
	}
	fn := func(ctx context.Context, _ domain.Project, r domain.Repository, _ types.RuntimeCLI) RepoResult {
		return RepoResult{Payload: r.Name}
	}

	groups, err := collectReport(context.Background(), project, testDeps(t, 2).RuntimeCLI, "testing", fn,
		NewReporter(&bytes.Buffer{}))
	if err != nil {
		t.Fatalf("unexpected interruption error: %v", err)
	}
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}
	if groups[0].Project.Name != "root" || groups[1].Project.Name != "sub" {
		t.Fatalf("group order = %q, %q", groups[0].Project.Name, groups[1].Project.Name)
	}
	var names []string
	for _, g := range groups {
		for _, res := range g.Results {
			if res == nil {
				t.Fatal("unexpected nil result without cancellation")
			}
			names = append(names, res.Payload.(string))
		}
	}
	want := []string{"r1", "r2", "s1"}
	if fmt.Sprint(names) != fmt.Sprint(want) {
		t.Fatalf("payload order = %v, want %v", names, want)
	}
}

// TestSingle verifies the single-repo entry: the result line renders with no
// project header and the repo's error returns as-is, so warnings still
// downgrade the exit code at the root.
func TestSingle(t *testing.T) {
	project := domain.Project{Name: "proj", Repos: []domain.Repository{repo("solo")}}

	deps := testDeps(t, 1)
	err := Single(context.Background(), project, repo("solo"), deps.RuntimeCLI, echoFunc(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(deps.Result(), lineFor(repo("solo"))) {
		t.Fatalf("missing repo line:\n%s", deps.Result())
	}
	if strings.Contains(deps.Result(), "proj") {
		t.Fatalf("single-repo output must not print the project header:\n%s", deps.Result())
	}

	warnFn := func(context.Context, domain.Project, domain.Repository, types.RuntimeCLI) RepoResult {
		return RepoResult{Line: "warned", Err: types.NewWarning("skip me")}
	}
	err = Single(context.Background(), project, repo("solo"), deps.RuntimeCLI, warnFn)
	if !types.IsWarning(err) {
		t.Fatalf("warning not passed through: %v", err)
	}
}

// TestSingleCollect verifies the grouped single-repo shape used by status: one
// group holding one result, plus the repo's error.
func TestSingleCollect(t *testing.T) {
	project := domain.Project{Name: "proj", Repos: []domain.Repository{repo("solo")}}
	fn := func(_ context.Context, _ domain.Project, r domain.Repository, _ types.RuntimeCLI) RepoResult {
		return RepoResult{Payload: r.Name, Err: fmt.Errorf("boom")}
	}

	groups, err := SingleCollect(context.Background(), project, repo("solo"), testDeps(t, 1).RuntimeCLI, fn)
	if err == nil || err.Error() != "boom" {
		t.Fatalf("err = %v, want boom", err)
	}
	if len(groups) != 1 || len(groups[0].Results) != 1 {
		t.Fatalf("groups shape = %+v, want 1 group with 1 result", groups)
	}
	if groups[0].Project.Name != "proj" || groups[0].Results[0].Payload != "solo" {
		t.Fatalf("group content = %+v", groups[0])
	}
}
