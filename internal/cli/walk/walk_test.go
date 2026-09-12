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
	"github.com/rafi/gits/internal/cli/config"
	"github.com/rafi/gits/internal/types"
)

// testDeps builds a RuntimeCLI with the default theme and the given worker
// count, enough to drive Walk without touching a real git client.
func testDeps(workers int) types.RuntimeCLI {
	return types.RuntimeCLI{
		Theme: config.NewThemeDefault(),
		Runtime: types.Runtime{
			Ctx:      context.Background(),
			Settings: domain.Settings{WorkerCount: workers},
		},
	}
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

	var out, prog bytes.Buffer
	errs := walkTo(context.Background(), project, testDeps(4), "testing", fn, &out, &prog)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}

	got := out.String()
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
	var out, prog bytes.Buffer
	walkTo(context.Background(), project, testDeps(2), "testing", echoFunc(nil), &out, &prog)

	got := out.String()
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
	walkTo(context.Background(), project, testDeps(4), "testing", fn, &bytes.Buffer{}, &bytes.Buffer{})
	if elapsed := time.Since(start); elapsed > 600*time.Millisecond {
		t.Fatalf("walk stalled: took %s, expected well under serial 960ms", elapsed)
	}
}

// TestWalkWorkerClamp verifies AC-2: WorkerCount <= 0 runs with one worker and
// never panics on a modulo or zero-sized pool.
func TestWalkWorkerClamp(t *testing.T) {
	project := domain.Project{Name: "proj", Repos: []domain.Repository{repo("a"), repo("b")}}
	var out bytes.Buffer
	errs := walkTo(context.Background(), project, testDeps(0), "testing", echoFunc(nil), &out, &bytes.Buffer{})
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if !strings.Contains(out.String(), lineFor(repo("a"))) || !strings.Contains(out.String(), lineFor(repo("b"))) {
		t.Fatalf("clamped walk did not process all repos:\n%s", out.String())
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
	errs := walkTo(context.Background(), project, testDeps(2), "testing", fn, &bytes.Buffer{}, &bytes.Buffer{})
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

	done := make(chan []error, 1)
	go func() {
		done <- walkTo(ctx, project, testDeps(2), "testing", fn, &bytes.Buffer{}, &bytes.Buffer{})
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

	groups, err := collectReport(ctx, project, testDeps(2), "testing", fn, &nopReporter{})
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
	groups, err = collectReport(context.Background(), project, testDeps(2), "testing",
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
	lastErrors          int
}

func (r *fakeReporter) Start(string, int, int) {}
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
func (r *fakeReporter) SetErrors(n int) {
	r.mu.Lock()
	r.lastErrors = n
	r.mu.Unlock()
}
func (r *fakeReporter) Stop() {}

type fakeRepoTracker struct{ r *fakeReporter }

func (t *fakeRepoTracker) Phase(string)     {}
func (t *fakeRepoTracker) SetTotal(int64)   {}
func (t *fakeRepoTracker) SetCurrent(int64) {}
func (t *fakeRepoTracker) Indeterminate()   {}
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

// orderSensitiveReporter records the last error count it finishes processing.
// SetErrors sleeps *inversely* to n (larger counts return sooner), so if the
// walker delivers counts after releasing its lock, a larger count can land
// before a smaller one and the final recorded value regresses below the true
// failure total. Delivering counts under the lock keeps them monotonic.
type orderSensitiveReporter struct {
	peak       int // highest count expected; sets the inverse sleep scale
	mu         sync.Mutex
	lastErrors int
}

func (r *orderSensitiveReporter) Start(string, int, int)       {}
func (r *orderSensitiveReporter) RepoStart(string) RepoTracker { return nopRepoTracker{} }
func (r *orderSensitiveReporter) Done()                        {}
func (r *orderSensitiveReporter) SetErrors(n int) {
	// Larger counts return faster; absent serialized delivery the smaller
	// count lands last and wins, regressing the displayed total.
	if d := time.Duration(r.peak-n) * 5 * time.Millisecond; d > 0 {
		time.Sleep(d)
	}
	r.mu.Lock()
	r.lastErrors = n
	r.mu.Unlock()
}
func (r *orderSensitiveReporter) Stop() {}
func (r *orderSensitiveReporter) Last() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lastErrors
}

// TestWalkErrorCountMonotonic verifies T4 (#5): with N concurrent failures, the
// final reported error count equals N regardless of delivery order. The walker
// must fold the count into the reporter while holding its lock so a late worker
// can't settle the spinner on a stale, smaller value.
func TestWalkErrorCountMonotonic(t *testing.T) {
	const failures = 8
	repos := make([]domain.Repository, failures)
	for i := range repos {
		repos[i] = repo(fmt.Sprintf("boom%02d", i))
	}
	project := domain.Project{Name: "proj", Repos: repos}
	fn := func(_ context.Context, _ domain.Project, r domain.Repository, _ types.RuntimeCLI) RepoResult {
		return RepoResult{Line: lineFor(r), Err: fmt.Errorf("real failure")}
	}
	rep := &orderSensitiveReporter{peak: failures}
	// One worker per repo: all failures race to deliver their count at once.
	errs := walkReport(context.Background(), project, testDeps(failures), "testing", fn, &bytes.Buffer{}, rep)
	if len(errs) != failures {
		t.Fatalf("aggregated errors = %d, want %d", len(errs), failures)
	}
	if got := rep.Last(); got != failures {
		t.Fatalf("final SetErrors = %d, want %d (count regressed under concurrent delivery)", got, failures)
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
	walkReport(context.Background(), project, testDeps(2), "testing", fn, &bytes.Buffer{}, rep)

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
	if rep.lastErrors != 1 {
		t.Errorf("lastErrors = %d, want 1", rep.lastErrors)
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

	groups, err := collectReport(context.Background(), project, testDeps(2), "testing", fn, NewReporter(&bytes.Buffer{}))
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
