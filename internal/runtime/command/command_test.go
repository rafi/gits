package command

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rafi/gits/domain"
	coreruntime "github.com/rafi/gits/internal/runtime"
)

// Every test here drives Run — the engine's entry point — with an explicit
// target, since resolving arguments is another package's job now. They cover
// the invariants no single command exercises; what each command does with the
// engine is asserted in that command's own tests.

// runtime builds a runtime with the given worker count. There is no output
// destination to give it: the engine renders nothing, which is why every
// assertion here is on the results it hands back.
func runtime(t *testing.T, workers int) coreruntime.Runtime {
	t.Helper()
	return coreruntime.Runtime{
		Ctx:      t.Context(),
		Settings: domain.Settings{WorkerCount: workers},
	}
}

// project returns a project named name carrying one ok repository per name
// given.
func project(name string, repos ...string) domain.Project {
	p := domain.Project{Name: name, Path: "/code/" + name}
	for _, r := range repos {
		p.Repos = append(p.Repos, repo(r, domain.RepoStateOK))
	}
	return p
}

// repo returns one repository in the given state.
func repo(name string, state domain.RepoState) domain.Repository {
	return domain.Repository{
		Name:    name,
		Dir:     name,
		AbsPath: "/code/" + name,
		State:   state,
	}
}

// cloned names n repositories r000….
func cloned(n int) []string {
	names := make([]string, n)
	for i := range names {
		names[i] = fmt.Sprintf("r%03d", i)
	}
	return names
}

// echo is a body that echoes the repository's name, optionally sleeping first.
func echo(delay func(string) time.Duration) func(
	context.Context, Repo, coreruntime.Runtime,
) (string, error) {
	return func(ctx context.Context, r Repo, _ coreruntime.Runtime) (string, error) {
		if delay != nil {
			if d := delay(r.GetName()); d > 0 {
				select {
				case <-time.After(d):
				case <-ctx.Done():
				}
			}
		}
		return lineFor(r.GetName()), nil
	}
}

// lineFor is a stable, greppable result body for a repository.
func lineFor(name string) string { return "LINE:" + name }

// values returns every result's value, in the order they came back.
func values[T any](res Results[T]) []T {
	out := make([]T, 0, len(res.Results))
	for _, r := range res.Results {
		out = append(out, r.Value)
	}
	return out
}

// TestRunStableOrder: results come back in tree order regardless of completion
// order. The repositories complete in reverse (later names finish first) but
// must still be reported alphabetically.
func TestRunStableOrder(t *testing.T) {
	t.Parallel()

	order := map[string]time.Duration{
		"alpha":   80 * time.Millisecond,
		"bravo":   60 * time.Millisecond,
		"charlie": 40 * time.Millisecond,
		"delta":   20 * time.Millisecond,
	}
	p := project("proj", "alpha", "bravo", "charlie", "delta")

	cmd := Command[string]{
		Verb: "testing",
		Do:   echo(func(name string) time.Duration { return order[name] }),
	}
	got := cmd.Run(Target{Project: p}, runtime(t, 4))

	want := []string{lineFor("alpha"), lineFor("bravo"), lineFor("charlie"), lineFor("delta")}
	if fmt.Sprint(values(got)) != fmt.Sprint(want) {
		t.Errorf("values = %v, want %v", values(got), want)
	}
}

// TestRunSubProjectOrder: the tree is walked depth-first, a project's own
// repositories before its sub-projects, the results come back in that order
// with their owning project, and the tree the run visited comes back whole.
func TestRunSubProjectOrder(t *testing.T) {
	t.Parallel()

	root := project("root", "r1", "r2")
	root.SubProjects = []domain.Project{project("sub", "s1")}

	cmd := Command[string]{Name: "test", Verb: "testing", Do: echo(nil)}
	got := cmd.Run(Target{Project: root}, runtime(t, 2))

	if want := []string{lineFor("r1"), lineFor("r2"), lineFor("s1")}; fmt.Sprint(values(got)) != fmt.Sprint(want) {
		t.Fatalf("values = %v, want %v", values(got), want)
	}
	if got.Project.Name != "root" || len(got.Project.SubProjects) != 1 {
		t.Fatalf("Project = %+v, want the whole tree the run visited", got.Project)
	}
	if got.Command != "test" {
		t.Errorf("Command = %q, want the command's Name carried on the results", got.Command)
	}
	var owners, keys []string
	for _, res := range got.Results {
		owners = append(owners, res.Repo.Project.Name)
		keys = append(keys, res.Repo.ProjectKey)
	}
	if want := []string{"root", "root", "sub"}; fmt.Sprint(owners) != fmt.Sprint(want) {
		t.Fatalf("owning projects = %v, want %v", owners, want)
	}
	if keys[0] != keys[1] || keys[2] == keys[0] {
		t.Errorf("project keys = %v, want the two root repositories grouped apart from the sub", keys)
	}
}

// TestRunNoStall: a free worker immediately picks the next task, so total
// wall-clock is bounded well under the fully serial sum.
func TestRunNoStall(t *testing.T) {
	t.Parallel()

	p := project("proj", append([]string{"slow"}, cloned(19)...)...)

	cmd := Command[string]{
		Verb: "testing",
		Do: echo(func(name string) time.Duration {
			if name == "slow" {
				return 200 * time.Millisecond
			}
			return 40 * time.Millisecond
		}),
	}

	// Serial sum = 200 + 19*40 = 960ms; a pool of 4 finishes well under 600ms.
	start := time.Now()
	cmd.Run(Target{Project: p}, runtime(t, 4))
	if elapsed := time.Since(start); elapsed > 600*time.Millisecond {
		t.Fatalf("run stalled: took %s, expected well under serial 960ms", elapsed)
	}
}

// TestRunWorkerClamp: a worker count at or below zero runs with one worker and
// never panics on a zero-sized pool.
func TestRunWorkerClamp(t *testing.T) {
	t.Parallel()

	cmd := Command[string]{Verb: "testing", Do: echo(nil)}
	got := cmd.Run(Target{Project: project("proj", "a", "b")}, runtime(t, 0))

	if want := []string{lineFor("a"), lineFor("b")}; fmt.Sprint(values(got)) != fmt.Sprint(want) {
		t.Fatalf("clamped run = %v, want all repositories processed", values(got))
	}
}

// TestRunProgressLifecycle: the injected reporter is begun with the verb and
// the total, started once per repository, finished with whether it failed —
// warnings are not failures — and stopped before the results are handed back.
func TestRunProgressLifecycle(t *testing.T) {
	t.Parallel()

	p := project("proj", "fine", "warn", "boom")
	spy := &spyProgress{}
	cmd := Command[string]{
		Verb:     "testing",
		Progress: spy,
		Do: func(_ context.Context, r Repo, _ coreruntime.Runtime) (string, error) {
			switch r.GetName() {
			case "warn":
				return "", domain.NewWarning("just a warning")
			case "boom":
				return "", errors.New("real failure")
			default:
				return lineFor("fine"), nil
			}
		},
	}
	cmd.Run(Target{Project: p}, runtime(t, 2))

	if spy.verb != "testing" || spy.total != 3 {
		t.Errorf("Begin(%q, %d), want (testing, 3)", spy.verb, spy.total)
	}
	if got := spy.started.Load(); got != 3 {
		t.Errorf("Start called %d times, want one per repository", got)
	}
	if got := spy.failed.Load(); got != 1 {
		t.Errorf("failures counted = %d, want only the real failure", got)
	}
	if !spy.stopped.Load() {
		t.Error("Stop was not called, want the reporter drained before results are handed back")
	}
}

// TestRunNoProgressIsSilent: a command that supplies no reporter runs to
// completion rather than dereferencing a nil interface.
func TestRunNoProgressIsSilent(t *testing.T) {
	t.Parallel()

	cmd := Command[string]{Verb: "testing", Do: echo(nil)}
	if got := cmd.Run(Target{Project: project("proj", "a")}, runtime(t, 1)); len(got.Results) != 1 {
		t.Fatalf("results = %+v, want the run to complete without a reporter", got.Results)
	}
}

// spyProgress records the lifecycle the engine drives it through.
type spyProgress struct {
	verb    string
	total   int
	started atomic.Int64
	failed  atomic.Int64
	stopped atomic.Bool
}

func (s *spyProgress) Begin(verb string, total int) { s.verb, s.total = verb, total }

func (s *spyProgress) Start(string) func(bool) {
	s.started.Add(1)
	return func(failed bool) {
		if failed {
			s.failed.Add(1)
		}
	}
}

func (s *spyProgress) Stop() { s.stopped.Store(true) }

// TestRunCancellation: canceling mid-run leaves queued repositories unstarted,
// returns promptly, and reports how many were never processed — instead of
// reporting partial success.
func TestRunCancellation(t *testing.T) {
	t.Parallel()

	const total = 40
	p := project("proj", cloned(total)...)

	ctx, cancel := context.WithCancel(t.Context())
	rt := runtime(t, 2)
	rt.Ctx = ctx

	var started atomic.Int64
	cmd := Command[string]{
		Verb: "testing",
		Do: func(c context.Context, r Repo, _ coreruntime.Runtime) (string, error) {
			if started.Add(1) == 4 {
				cancel() // trip cancellation a few tasks in
			}
			select {
			case <-time.After(50 * time.Millisecond):
			case <-c.Done():
			}
			return lineFor(r.GetName()), nil
		},
	}

	done := make(chan Results[string], 1)
	go func() { done <- cmd.Run(Target{Project: p}, rt) }()
	var got Results[string]
	select {
	case got = <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("run did not return promptly after cancellation")
	}

	if s := started.Load(); s >= total {
		t.Fatalf("cancellation did not stop queued repositories: started=%d total=%d", s, total)
	}
	// The repositories never started have no result at all — nothing for a
	// renderer to skip over — and the interruption accounts for them.
	if n := len(got.Results); n >= total || n != int(started.Load()) {
		t.Errorf("results = %d, want exactly the %d repositories that started", n, started.Load())
	}
	if got.Interrupted == nil ||
		!strings.Contains(got.Interrupted.Error(), fmt.Sprintf("%d of %d", total-len(got.Results), total)) {
		t.Errorf("Interrupted = %v, want the unstarted count named", got.Interrupted)
	}
}

// TestRunSingleRepository covers the single-repository dispatch: only the
// named repository runs, and the tree the run reports is the project narrowed
// to it.
func TestRunSingleRepository(t *testing.T) {
	t.Parallel()

	p := project("proj", "api", "web")
	api := p.Repos[0]

	var reached []string
	cmd := Command[string]{
		Verb: "testing",
		Do: func(_ context.Context, r Repo, _ coreruntime.Runtime) (string, error) {
			reached = append(reached, r.GetName())
			return lineFor(r.GetName()), domain.NewWarning("skip me")
		},
	}
	got := cmd.Run(Target{Project: p, Repo: &api}, runtime(t, 1))

	if fmt.Sprint(reached) != fmt.Sprint([]string{"api"}) {
		t.Errorf("body reached %v, want only the named repository", reached)
	}
	if len(got.Project.Repos) != 1 || got.Project.Repos[0].Name != "api" || len(got.Results) != 1 {
		t.Errorf("Results = %+v, want the project narrowed to the named repository", got)
	}
	if err := got.Results[0].Err; !domain.IsWarning(err) {
		t.Errorf("error = %v, want the body's warning kept as it is", err)
	}
}

// TestRunStateGuard: a nil accepted-states set means the ok state, and a
// repository failing the guard carries its state error and never reaches the
// body.
func TestRunStateGuard(t *testing.T) {
	t.Parallel()

	p := domain.Project{Name: "proj", Repos: []domain.Repository{
		repo("api", domain.RepoStateOK),
		repo("gone", domain.RepoStateNotCloned),
		repo("bad", domain.RepoStateError),
	}}

	var reached []string
	cmd := Command[string]{
		Verb: "testing",
		Do: func(_ context.Context, r Repo, _ coreruntime.Runtime) (string, error) {
			reached = append(reached, r.GetName())
			return lineFor(r.GetName()), nil
		},
	}
	got := cmd.Run(Target{Project: p}, runtime(t, 2))

	if fmt.Sprint(reached) != fmt.Sprint([]string{"api"}) {
		t.Errorf("body reached %v, want only the ok repository", reached)
	}
	// A turned-back repository says so on its result, so a renderer can tell
	// "never tried" from "tried and failed" without re-deriving the guard.
	for _, r := range got.Results {
		want := r.Repo.GetName() != "api"
		if r.Guarded != want {
			t.Errorf("%s Guarded = %v, want %v", r.Repo.GetName(), r.Guarded, want)
		}
		if want && r.Err == nil {
			t.Errorf("%s Err = nil, want its state error", r.Repo.GetName())
		}
	}
	if err := got.Results[1].Err; !errors.Is(err, ErrNotCloned) {
		t.Errorf("gone error = %v, want %v", err, ErrNotCloned)
	}
}

// TestRunAcceptsWidensTheGuard: a command declaring the states it acts on
// receives repositories in them, and still turns back the ones it did not
// declare.
func TestRunAcceptsWidensTheGuard(t *testing.T) {
	t.Parallel()

	p := domain.Project{Name: "proj", Repos: []domain.Repository{
		repo("gone", domain.RepoStateNotCloned),
		repo("bad", domain.RepoStateError),
	}}

	var reached []string
	cmd := Command[string]{
		Verb:    "testing",
		Accepts: []domain.RepoState{domain.RepoStateOK, domain.RepoStateNotCloned},
		Do: func(_ context.Context, r Repo, _ coreruntime.Runtime) (string, error) {
			reached = append(reached, r.GetName())
			return lineFor(r.GetName()), nil
		},
	}
	got := cmd.Run(Target{Project: p}, runtime(t, 2))

	if fmt.Sprint(reached) != fmt.Sprint([]string{"gone"}) {
		t.Errorf("body reached %v, want the declared state only", reached)
	}
	if !got.Results[1].Guarded {
		t.Errorf("bad = %+v, want the undeclared state turned back", got.Results[1])
	}
}

// TestRunKeepsBodyErrorsBare: a body's error is kept exactly as returned, so a
// renderer decides how to present it.
func TestRunKeepsBodyErrorsBare(t *testing.T) {
	t.Parallel()

	p := project("proj", "api")
	boom := errors.New("boom")
	cmd := Command[string]{
		Verb: "testing",
		Do:   func(context.Context, Repo, coreruntime.Runtime) (string, error) { return "", boom },
	}
	got := cmd.Run(Target{Project: p}, runtime(t, 1))

	if err := got.Results[0].Err; err != boom { //nolint:errorlint // identity is the assertion
		t.Fatalf("body error = %v, want it kept as returned", err)
	}
}

// TestRunSkipsProjects: a skipped project vanishes — never queued, never
// reported — on both dispatch paths, and is named on the results so a view can
// say so.
func TestRunSkipsProjects(t *testing.T) {
	t.Parallel()

	p := project("proj", "api", "web")
	api := p.Repos[0]

	for _, tc := range []struct {
		name   string
		target Target
	}{
		{"whole tree", Target{Project: p}},
		{"named repository", Target{Project: p, Repo: &api}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var reached []string
			cmd := Command[string]{
				Verb: "testing",
				Skip: func(p domain.Project) bool { return p.Name == "proj" },
				Do: func(_ context.Context, r Repo, _ coreruntime.Runtime) (string, error) {
					reached = append(reached, r.GetName())
					return lineFor(r.GetName()), nil
				},
			}
			got := cmd.Run(tc.target, runtime(t, 2))

			if len(reached) != 0 {
				t.Errorf("body reached %v, want nothing under a skipped project", reached)
			}
			if len(got.Results) != 0 || got.Project.Name != "" {
				t.Errorf("Results = %+v, want nothing for a skipped project", got)
			}
			if fmt.Sprint(got.Skipped) != fmt.Sprint([]string{"proj"}) {
				t.Errorf("Skipped = %v, want the skipped project named", got.Skipped)
			}
		})
	}
}

// TestRunSkipsSubProjects: the skip reaches a sub-project too, taking
// everything beneath it — and naming a repository inside one explicitly does
// not override it, which is the defect the single-repository path used to
// carry by branching before the pruning step (ADR-0005).
func TestRunSkipsSubProjects(t *testing.T) {
	t.Parallel()

	root := project("root", "api")
	vendor := project("vendor", "forks")
	vendor.SubProjects = []domain.Project{project("deep", "inner")}
	root.SubProjects = []domain.Project{vendor}
	forks := vendor.Repos[0]

	for _, tc := range []struct {
		name   string
		target Target
		want   []string
	}{
		{"whole tree", Target{Project: root}, []string{"api"}},
		{"named repository under the skip", Target{Project: root, Repo: &forks}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var reached []string
			cmd := Command[string]{
				Verb: "testing",
				Skip: func(p domain.Project) bool { return p.Name == "vendor" },
				Do: func(_ context.Context, r Repo, _ coreruntime.Runtime) (string, error) {
					reached = append(reached, r.GetName())
					return lineFor(r.GetName()), nil
				},
			}
			got := cmd.Run(tc.target, runtime(t, 2))

			if fmt.Sprint(reached) != fmt.Sprint(tc.want) {
				t.Errorf("body reached %v, want %v", reached, tc.want)
			}
			if len(got.Project.SubProjects) != 0 {
				t.Errorf("tree = %+v, want the skipped sub-project dropped whole", got.Project)
			}
			if fmt.Sprint(got.Skipped) != fmt.Sprint([]string{"vendor"}) {
				t.Errorf("Skipped = %v, want only the skipped sub-project named", got.Skipped)
			}
		})
	}
}

// TestRunCollectsTypedResults: a renderer receives the body's own type, so no
// caller asserts a type at this seam and no row is dropped when such an
// assertion would have failed.
func TestRunCollectsTypedResults(t *testing.T) {
	t.Parallel()

	type probe struct{ name string }

	cmd := Command[*probe]{
		Verb: "testing",
		Do: func(_ context.Context, r Repo, _ coreruntime.Runtime) (*probe, error) {
			return &probe{name: r.GetName()}, nil
		},
	}
	got := values(cmd.Run(Target{Project: project("proj", "api", "web")}, runtime(t, 2)))

	if len(got) != 2 || got[0].name != "api" || got[1].name != "web" {
		t.Fatalf("collected = %+v, want both probes in tree order", got)
	}
}

// TestRunHandsDisplayPaths: every body receives its repository's display path,
// with the home directory shortened, so a renderer never re-derives it.
func TestRunHandsDisplayPaths(t *testing.T) {
	t.Parallel()

	p := domain.Project{Name: "proj", AbsPath: "/code/proj", Repos: []domain.Repository{
		{Name: "api", AbsPath: "/home/nobody/code/api", State: domain.RepoStateOK},
		{Name: "web", Dir: "much-longer-name", State: domain.RepoStateOK},
	}}

	var paths []string
	cmd := Command[string]{
		Verb: "testing",
		Do: func(_ context.Context, r Repo, _ coreruntime.Runtime) (string, error) {
			paths = append(paths, r.Path)
			return lineFor(r.GetName()), nil
		},
	}
	rt := runtime(t, 1)
	rt.HomeDir = "/home/nobody"
	cmd.Run(Target{Project: p}, rt)

	if want := []string{"~/code/api", "much-longer-name"}; fmt.Sprint(paths) != fmt.Sprint(want) {
		t.Fatalf("paths = %v, want %v", paths, want)
	}
}
