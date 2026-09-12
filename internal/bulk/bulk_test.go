package bulk

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cli/clitest"
	"github.com/rafi/gits/internal/types"
)

// Every test here drives Run — the module's real entry point — with explicit
// arguments, so argument resolution, pruning, the dispatch between a whole
// tree and a single repository, the state guard, the worker pool and the
// renderer all run for real, and the interactive finder is never reached.
//
// They cover the invariants no single Bulk Command exercises; what each
// command does with the module is asserted in that command's own tests.

// body is the per-repository function a Command declares.
type body = func(context.Context, Repo, types.RuntimeCLI) (string, error)

// deps builds test dependencies with the given worker count. Both output
// destinations are buffers, so Result Output and Diagnostic Output are
// captured separately and the progress reporter falls silent.
func deps(t *testing.T, workers int, repos ...clitest.Repo) *clitest.Deps {
	t.Helper()
	d := clitest.New(t, clitest.FakeGit{}).WithProject("proj", repos...)
	d.Settings.WorkerCount = workers
	return d
}

// cloned declares n repositories named r000…, all in Repo State ok.
func cloned(n int) []clitest.Repo {
	repos := make([]clitest.Repo, n)
	for i := range repos {
		repos[i] = clitest.Cloned(fmt.Sprintf("r%03d", i))
	}
	return repos
}

// echo is a body that echoes the repository's name, optionally sleeping first.
func echo(delay func(string) time.Duration) body {
	return func(ctx context.Context, repo Repo, _ types.RuntimeCLI) (string, error) {
		if delay != nil {
			if d := delay(repo.GetName()); d > 0 {
				select {
				case <-time.After(d):
				case <-ctx.Done():
				}
			}
		}
		return lineFor(repo.GetName()), nil
	}
}

// lineFor is a stable, greppable result body for a repository.
func lineFor(name string) string { return "LINE:" + name }

// inOrder asserts that every want appears in got, in the order given.
func inOrder(t *testing.T, got string, want ...string) {
	t.Helper()
	last := -1
	for _, w := range want {
		i := strings.Index(got, w)
		if i < 0 {
			t.Fatalf("missing %q in output:\n%s", w, got)
		}
		if i < last {
			t.Fatalf("out of order: %q appears before a prior entry:\n%s", w, got)
		}
		last = i
	}
}

// TestRunStableOrder: results render in tree order regardless of completion
// order. The repositories complete in reverse (later names finish first) but
// must still print alphabetically under the project title.
func TestRunStableOrder(t *testing.T) {
	t.Parallel()

	order := map[string]time.Duration{
		"alpha":   80 * time.Millisecond,
		"bravo":   60 * time.Millisecond,
		"charlie": 40 * time.Millisecond,
		"delta":   20 * time.Millisecond,
	}
	d := deps(t, 4, clitest.Cloned("alpha"), clitest.Cloned("bravo"),
		clitest.Cloned("charlie"), clitest.Cloned("delta"))

	cmd := Command[string]{
		Verb:   "testing",
		Body:   echo(func(name string) time.Duration { return order[name] }),
		Render: Lines,
	}
	if err := cmd.Run([]string{"proj"}, d.RuntimeCLI); err != nil {
		t.Fatalf("Run error = %v, want nil", err)
	}

	inOrder(t, d.Result(), lineFor("alpha"), lineFor("bravo"),
		lineFor("charlie"), lineFor("delta"))
}

// TestRunSubProjectOrder: the tree is walked depth-first, a project's own
// repositories before its sub-projects, and the groups a renderer receives
// carry that same order — the contract status's JSON renderer inverts
// positionally to rebuild the tree.
func TestRunSubProjectOrder(t *testing.T) {
	t.Parallel()

	d := clitest.New(t, clitest.FakeGit{})
	root := clitest.NewProject(t, clitest.Cloned("r1"), clitest.Cloned("r2"))
	sub := clitest.NewProject(t, clitest.Cloned("s1"))
	sub.Name = "sub"
	root.SubProjects = []domain.Project{sub}
	d.Projects["root"] = root

	var got Results[string]
	cmd := Command[string]{
		Verb: "testing",
		Body: echo(nil),
		Render: func(res Results[string], deps types.RuntimeCLI) error {
			got = res
			return Lines(res, deps)
		},
	}
	if err := cmd.Run([]string{"root"}, d.RuntimeCLI); err != nil {
		t.Fatalf("Run error = %v, want nil", err)
	}

	inOrder(t, d.Result(), "root", lineFor("r1"), lineFor("r2"), "sub", lineFor("s1"))

	if len(got.Groups) != 2 {
		t.Fatalf("groups = %d, want one per project node", len(got.Groups))
	}
	if got.Groups[0].Project.Name != "root" || got.Groups[1].Project.Name != "sub" {
		t.Fatalf("group order = %q, %q, want root, sub",
			got.Groups[0].Project.Name, got.Groups[1].Project.Name)
	}
	if got.Single {
		t.Error("Single = true for a whole-tree run")
	}
	var names []string
	for _, g := range got.Groups {
		for _, res := range g.Results {
			if res == nil {
				t.Fatal("unexpected nil result without cancellation")
			}
			names = append(names, res.Value)
		}
	}
	want := []string{lineFor("r1"), lineFor("r2"), lineFor("s1")}
	if fmt.Sprint(names) != fmt.Sprint(want) {
		t.Fatalf("collected values = %v, want %v", names, want)
	}
}

// TestRunUsesDependencyDestinations proves the module writes where its
// dependencies say: every rendered line is Result Output, and a non-terminal
// Diagnostic Output yields the no-op reporter, so no progress or ANSI cursor
// control can reach a captured run's Result Output.
func TestRunUsesDependencyDestinations(t *testing.T) {
	t.Parallel()

	d := deps(t, 2, clitest.Cloned("a"), clitest.Cloned("b"))

	cmd := Command[string]{Verb: "testing", Body: echo(nil), Render: Lines}
	if err := cmd.Run([]string{"proj"}, d.RuntimeCLI); err != nil {
		t.Fatalf("Run error = %v, want nil", err)
	}

	for _, want := range []string{lineFor("a"), lineFor("b")} {
		if !strings.Contains(d.Result(), want) {
			t.Fatalf("Result Output missing %q:\n%s", want, d.Result())
		}
	}
	if diag := d.Diagnostic(); diag != "" {
		t.Fatalf("Diagnostic Output should be silent without a terminal, got:\n%q", diag)
	}
}

// TestRunNoStall: a free worker immediately picks the next task, so total
// wall-clock is bounded well under the fully serial sum.
func TestRunNoStall(t *testing.T) {
	t.Parallel()

	repos := append([]clitest.Repo{clitest.Cloned("slow")}, cloned(19)...)
	d := deps(t, 4, repos...)

	cmd := Command[string]{
		Verb: "testing",
		Body: echo(func(name string) time.Duration {
			if name == "slow" {
				return 200 * time.Millisecond
			}
			return 40 * time.Millisecond
		}),
		Render: Lines,
	}

	// Serial sum = 200 + 19*40 = 960ms; a pool of 4 finishes well under 600ms.
	start := time.Now()
	if err := cmd.Run([]string{"proj"}, d.RuntimeCLI); err != nil {
		t.Fatalf("Run error = %v, want nil", err)
	}
	if elapsed := time.Since(start); elapsed > 600*time.Millisecond {
		t.Fatalf("run stalled: took %s, expected well under serial 960ms", elapsed)
	}
}

// TestRunWorkerClamp: a worker count at or below zero runs with one worker and
// never panics on a zero-sized pool.
func TestRunWorkerClamp(t *testing.T) {
	t.Parallel()

	d := deps(t, 0, clitest.Cloned("a"), clitest.Cloned("b"))

	cmd := Command[string]{Verb: "testing", Body: echo(nil), Render: Lines}
	if err := cmd.Run([]string{"proj"}, d.RuntimeCLI); err != nil {
		t.Fatalf("Run error = %v, want nil", err)
	}
	for _, want := range []string{lineFor("a"), lineFor("b")} {
		if !strings.Contains(d.Result(), want) {
			t.Fatalf("clamped run did not process all repositories:\n%s", d.Result())
		}
	}
}

// TestRunErrorAggregation: warnings are excluded from the aggregated failure
// count while real errors are reported, so a warning cannot fail the run.
func TestRunErrorAggregation(t *testing.T) {
	t.Parallel()

	d := deps(t, 2, clitest.Cloned("fine"), clitest.Cloned("warn"), clitest.Cloned("boom"))

	cmd := Command[string]{
		Verb: "testing",
		Body: func(_ context.Context, repo Repo, _ types.RuntimeCLI) (string, error) {
			switch repo.GetName() {
			case "warn":
				return lineFor("warn"), types.NewWarning("just a warning")
			case "boom":
				return "", errors.New("real failure")
			default:
				return lineFor("fine"), nil
			}
		},
		Render: Lines,
	}
	err := cmd.Run([]string{"proj"}, d.RuntimeCLI)
	if err == nil {
		t.Fatal("Run error = nil, want the real failure to fail the run")
	}

	diagnostic := d.Diagnostic()
	if !strings.Contains(diagnostic, "1 error:") {
		t.Errorf("Diagnostic Output = %q, want exactly the real failure counted", diagnostic)
	}
	if !strings.Contains(diagnostic, "real failure") {
		t.Errorf("Diagnostic Output = %q, want the failure's message", diagnostic)
	}
	if strings.Contains(diagnostic, "just a warning") {
		t.Errorf("Diagnostic Output = %q, want the warning kept out of the epilogue", diagnostic)
	}
	// The warning still shows on its own repository's line.
	if got := d.Result(); !strings.Contains(got, "just a warning") {
		t.Errorf("Result Output = %q, want the warning on the repository's line", got)
	}
}

// TestRunCancellation: canceling mid-run leaves queued repositories unstarted,
// returns promptly, and fails loudly naming how many were never processed —
// instead of reporting partial success.
func TestRunCancellation(t *testing.T) {
	t.Parallel()

	const total = 40
	d := deps(t, 2, cloned(total)...)

	ctx, cancel := context.WithCancel(t.Context())
	d.Ctx = ctx

	var started atomic.Int64
	cmd := Command[string]{
		Verb: "testing",
		Body: func(c context.Context, repo Repo, _ types.RuntimeCLI) (string, error) {
			if started.Add(1) == 4 {
				cancel() // trip cancellation a few tasks in
			}
			select {
			case <-time.After(50 * time.Millisecond):
			case <-c.Done():
			}
			return lineFor(repo.GetName()), nil
		},
		Render: Lines,
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Run([]string{"proj"}, d.RuntimeCLI) }()
	var err error
	select {
	case err = <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("run did not return promptly after cancellation")
	}

	if s := started.Load(); s >= total {
		t.Fatalf("cancellation did not stop queued repositories: started=%d total=%d", s, total)
	}
	if err == nil {
		t.Fatal("Run error = nil, want an interrupted run to fail")
	}
	diagnostic := d.Diagnostic()
	if !strings.Contains(diagnostic, "interrupted") ||
		!strings.Contains(diagnostic, fmt.Sprintf("of %d repositories", total)) {
		t.Errorf("Diagnostic Output = %q, want the interruption to name how many were skipped",
			diagnostic)
	}
}

// TestRunSingleRepository covers the single-repository dispatch: only the named
// repository runs, its line renders without a project title, and its error
// returns as itself — so a warning still downgrades the exit code at the root
// rather than being counted as a failure.
func TestRunSingleRepository(t *testing.T) {
	t.Parallel()

	d := deps(t, 1, clitest.Cloned("api"), clitest.Cloned("web"))

	var reached []string
	var single bool
	cmd := Command[string]{
		Verb: "testing",
		Body: func(_ context.Context, repo Repo, _ types.RuntimeCLI) (string, error) {
			reached = append(reached, repo.GetName())
			return lineFor(repo.GetName()), types.NewWarning("skip me")
		},
		Render: func(res Results[string], deps types.RuntimeCLI) error {
			single = res.Single
			return Lines(res, deps)
		},
	}

	err := cmd.Run([]string{"proj", "api"}, d.RuntimeCLI)
	if !types.IsWarning(err) {
		t.Fatalf("Run error = %v, want the repository's warning passed through", err)
	}
	if !single {
		t.Error("Single = false for a run naming one repository")
	}
	if fmt.Sprint(reached) != fmt.Sprint([]string{"api"}) {
		t.Errorf("body reached %v, want only the named repository", reached)
	}
	got := d.Result()
	if !strings.Contains(got, "api") || !strings.Contains(got, "skip me") {
		t.Errorf("Result Output = %q, want the named repository's line", got)
	}
	if strings.Contains(got, "proj") {
		t.Errorf("Result Output = %q, want no project title on the single path", got)
	}
}

// TestRunStateGuard: a nil accepted-states set means the ok state, and a
// repository failing the guard renders its state error and never reaches the
// body.
func TestRunStateGuard(t *testing.T) {
	t.Parallel()

	d := deps(t, 2, clitest.Cloned("api"), clitest.NotCloned("gone"), clitest.Broken("bad"))

	var reached []string
	cmd := Command[string]{
		Verb: "testing",
		Body: func(_ context.Context, repo Repo, _ types.RuntimeCLI) (string, error) {
			reached = append(reached, repo.GetName())
			return lineFor(repo.GetName()), nil
		},
		Render: Lines,
	}
	err := cmd.Run([]string{"proj"}, d.RuntimeCLI)
	if err == nil {
		t.Fatal("Run error = nil, want the guarded repositories to fail the run")
	}
	if fmt.Sprint(reached) != fmt.Sprint([]string{"api"}) {
		t.Errorf("body reached %v, want only the ok repository", reached)
	}
	for _, want := range []string{"not cloned", clitest.BrokenReason} {
		if got := d.Result(); !strings.Contains(got, want) {
			t.Errorf("Result Output = %q, want the guarded row to explain itself with %q",
				got, want)
		}
	}
	if got := d.Diagnostic(); !strings.Contains(got, "2 errors:") {
		t.Errorf("Diagnostic Output = %q, want both guarded repositories counted", got)
	}
}

// TestRunAcceptsWidensTheGuard: a command declaring the states it acts on
// receives repositories in them, and still turns back the ones it did not
// declare.
func TestRunAcceptsWidensTheGuard(t *testing.T) {
	t.Parallel()

	d := deps(t, 2, clitest.NotCloned("gone"), clitest.Broken("bad"))

	var reached []string
	cmd := Command[string]{
		Verb:    "testing",
		Accepts: []domain.RepoState{domain.RepoStateOK, domain.RepoStateNotCloned},
		Body: func(_ context.Context, repo Repo, _ types.RuntimeCLI) (string, error) {
			reached = append(reached, repo.GetName())
			return lineFor(repo.GetName()), nil
		},
		Render: Lines,
	}
	if err := cmd.Run([]string{"proj"}, d.RuntimeCLI); err == nil {
		t.Fatal("Run error = nil, want the undeclared state to fail the run")
	}
	if fmt.Sprint(reached) != fmt.Sprint([]string{"gone"}) {
		t.Errorf("body reached %v, want the declared state only", reached)
	}
}

// TestRunWrapsBodyErrors: a plain error is wrapped with the repository's name
// and path, so no body repeats that call; an error a body already wrapped as a
// warning passes through untouched, which is how a documented pass-over
// condition stays visibly different in the source.
func TestRunWrapsBodyErrors(t *testing.T) {
	t.Parallel()

	d := deps(t, 1, clitest.Cloned("api"))

	var got error
	capture := func(res Results[string], _ types.RuntimeCLI) error {
		got = res.Groups[0].Results[0].Err
		return nil
	}

	plain := Command[string]{
		Verb:   "testing",
		Body:   func(context.Context, Repo, types.RuntimeCLI) (string, error) { return "", errors.New("boom") },
		Render: capture,
	}
	if err := plain.Run([]string{"proj", "api"}, d.RuntimeCLI); err != nil {
		t.Fatalf("Run error = %v, want nil from the capturing renderer", err)
	}
	var wrapped *types.Warning
	if !errors.As(got, &wrapped) {
		t.Fatalf("body error = %v (%T), want it wrapped for the repository", got, got)
	}
	if wrapped.Title != "api" || wrapped.Type != types.ErrorType {
		t.Errorf("wrapped error = %+v, want the repository named and a real failure", wrapped)
	}

	warning := types.NewWarning("already handled")
	downgraded := Command[string]{
		Verb:   "testing",
		Body:   func(context.Context, Repo, types.RuntimeCLI) (string, error) { return "", warning },
		Render: capture,
	}
	if err := downgraded.Run([]string{"proj", "api"}, d.RuntimeCLI); err != nil {
		t.Fatalf("Run error = %v, want nil from the capturing renderer", err)
	}
	if !errors.Is(got, warning) || got != warning { //nolint:errorlint // identity is the assertion
		t.Errorf("body error = %v, want the already-wrapped warning left untouched", got)
	}
}

// TestRunSkipsProjects: a skipped project vanishes — never queued, never
// rendered — on both dispatch paths, and is named as Diagnostic Output so
// silence is not indistinguishable from an empty project.
func TestRunSkipsProjects(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{{"proj"}, {"proj", "api"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			t.Parallel()

			d := deps(t, 2, clitest.Cloned("api"), clitest.Cloned("web"))

			var reached []string
			cmd := Command[string]{
				Verb: "testing",
				Skip: func(p domain.Project) bool { return p.Name == "proj" },
				Body: func(_ context.Context, repo Repo, _ types.RuntimeCLI) (string, error) {
					reached = append(reached, repo.GetName())
					return lineFor(repo.GetName()), nil
				},
				Render: Lines,
			}
			if err := cmd.Run(args, d.RuntimeCLI); err != nil {
				t.Fatalf("Run error = %v, want nil", err)
			}
			if len(reached) != 0 {
				t.Errorf("body reached %v, want nothing under a skipped project", reached)
			}
			// The project vanishes rather than appearing as an empty block:
			// not even its title reaches Result Output.
			if got := d.Result(); got != "" {
				t.Errorf("Result Output = %q, want a skipped project to render nothing", got)
			}
			if got := d.Diagnostic(); !strings.Contains(got, "Skipping proj") {
				t.Errorf("Diagnostic Output = %q, want the skipped project named", got)
			}
		})
	}
}

// TestRunSkipsSubProjects: the skip reaches a sub-project too, taking
// everything beneath it — and naming a repository inside one explicitly does
// not override it, which is the defect the single-repository path used to
// carry by branching before the pruning step.
func TestRunSkipsSubProjects(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		args []string
		want []string
	}{
		{"whole tree", []string{"root"}, []string{"api"}},
		{"named repository under the skip", []string{"root", "vendor/forks"}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			d := clitest.New(t, clitest.FakeGit{})
			root := clitest.NewProject(t, clitest.Cloned("api"))
			vendor := clitest.NewProject(t, clitest.Cloned("forks"))
			vendor.Name = "vendor"
			root.SubProjects = []domain.Project{vendor}
			d.Projects["root"] = root

			var reached []string
			cmd := Command[string]{
				Verb: "testing",
				Skip: func(p domain.Project) bool { return p.Name == "vendor" },
				Body: func(_ context.Context, repo Repo, _ types.RuntimeCLI) (string, error) {
					reached = append(reached, repo.GetName())
					return lineFor(repo.GetName()), nil
				},
				Render: Lines,
			}
			if err := cmd.Run(tc.args, d.RuntimeCLI); err != nil {
				t.Fatalf("Run error = %v, want nil", err)
			}

			if fmt.Sprint(reached) != fmt.Sprint(tc.want) {
				t.Errorf("body reached %v, want %v", reached, tc.want)
			}
			result := d.Result()
			if strings.Contains(result, "vendor") || strings.Contains(result, lineFor("forks")) {
				t.Errorf("Result Output = %q, want the skipped sub-project to have vanished", result)
			}
			if got := d.Diagnostic(); !strings.Contains(got, "Skipping vendor") {
				t.Errorf("Diagnostic Output = %q, want the skipped sub-project named", got)
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

	d := deps(t, 2, clitest.Cloned("api"), clitest.Cloned("web"))

	var got []*probe
	cmd := Command[*probe]{
		Verb: "testing",
		Body: func(_ context.Context, repo Repo, _ types.RuntimeCLI) (*probe, error) {
			return &probe{name: repo.GetName()}, nil
		},
		Render: func(res Results[*probe], _ types.RuntimeCLI) error {
			for _, g := range res.Groups {
				for _, r := range g.Results {
					got = append(got, r.Value)
				}
			}
			return nil
		},
	}
	if err := cmd.Run([]string{"proj"}, d.RuntimeCLI); err != nil {
		t.Fatalf("Run error = %v, want nil", err)
	}
	if len(got) != 2 || got[0].name != "api" || got[1].name != "web" {
		t.Fatalf("collected = %+v, want both probes in tree order", got)
	}
}

// TestRunPadsTitles: every body receives its title already measured against
// the widest in its project, so no body computes one and none can compute it
// wrongly.
func TestRunPadsTitles(t *testing.T) {
	t.Parallel()

	d := deps(t, 1, clitest.Cloned("api"), clitest.Cloned("much-longer-name"))

	widths := map[string]int{}
	cmd := Command[string]{
		Verb: "testing",
		Body: func(_ context.Context, repo Repo, _ types.RuntimeCLI) (string, error) {
			widths[repo.Title.Value()] = repo.Title.GetWidth()
			return "", nil
		},
		Render: func(Results[string], types.RuntimeCLI) error { return nil },
	}
	if err := cmd.Run([]string{"proj"}, d.RuntimeCLI); err != nil {
		t.Fatalf("Run error = %v, want nil", err)
	}
	want := len("much-longer-name")
	if widths["api"] != want || widths["much-longer-name"] != want {
		t.Fatalf("title widths = %v, want both padded to %d", widths, want)
	}
}
