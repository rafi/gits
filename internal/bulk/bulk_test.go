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

// run drives cmd over args and renders the results with Lines, as the four
// line-rendering Bulk Commands do.
func run(cmd Command[string], args []string, deps types.RuntimeCLI) error {
	res, err := cmd.Run(args, deps)
	if err != nil {
		return err
	}
	return Lines(res, deps)
}

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
		Verb: "testing",
		Body: echo(func(name string) time.Duration { return order[name] }),
	}
	if err := run(cmd, []string{"proj"}, d.RuntimeCLI); err != nil {
		t.Fatalf("Run error = %v, want nil", err)
	}

	inOrder(t, d.Result(), lineFor("alpha"), lineFor("bravo"),
		lineFor("charlie"), lineFor("delta"))
}

// TestRunSubProjectOrder: the tree is walked depth-first, a project's own
// repositories before its sub-projects, the results come back in that order
// with their owning project, and the tree the run visited comes back whole.
func TestRunSubProjectOrder(t *testing.T) {
	t.Parallel()

	d := clitest.New(t, clitest.FakeGit{})
	root := clitest.NewProject(t, clitest.Cloned("r1"), clitest.Cloned("r2"))
	sub := clitest.NewProject(t, clitest.Cloned("s1"))
	sub.Name = "sub"
	root.SubProjects = []domain.Project{sub}
	d.Projects["root"] = root

	cmd := Command[string]{Verb: "testing", Body: echo(nil)}
	got, err := cmd.Run([]string{"root"}, d.RuntimeCLI)
	if err != nil {
		t.Fatalf("Run error = %v, want nil", err)
	}
	if err := Lines(got, d.RuntimeCLI); err != nil {
		t.Fatalf("Lines error = %v, want nil", err)
	}

	inOrder(t, d.Result(), lineFor("r1"), lineFor("r2"), lineFor("s1"))

	if got.Project.Name != "root" || len(got.Project.SubProjects) != 1 {
		t.Fatalf("Project = %+v, want the whole tree the run visited", got.Project)
	}
	var values, owners []string
	for _, res := range got.Results {
		values = append(values, res.Value)
		owners = append(owners, res.Repo.Project.Name)
	}
	if want := []string{lineFor("r1"), lineFor("r2"), lineFor("s1")}; fmt.Sprint(values) != fmt.Sprint(want) {
		t.Fatalf("collected values = %v, want %v", values, want)
	}
	if want := []string{"root", "root", "sub"}; fmt.Sprint(owners) != fmt.Sprint(want) {
		t.Fatalf("owning projects = %v, want %v", owners, want)
	}
}

// TestRunUsesDependencyDestinations proves the module writes where its
// dependencies say: every rendered line is Result Output, and a non-terminal
// Diagnostic Output yields the no-op reporter, so no progress or ANSI cursor
// control can reach a captured run's Result Output.
func TestRunUsesDependencyDestinations(t *testing.T) {
	t.Parallel()

	d := deps(t, 2, clitest.Cloned("a"), clitest.Cloned("b"))

	cmd := Command[string]{Verb: "testing", Body: echo(nil)}
	if err := run(cmd, []string{"proj"}, d.RuntimeCLI); err != nil {
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
	}

	// Serial sum = 200 + 19*40 = 960ms; a pool of 4 finishes well under 600ms.
	start := time.Now()
	if err := run(cmd, []string{"proj"}, d.RuntimeCLI); err != nil {
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

	cmd := Command[string]{Verb: "testing", Body: echo(nil)}
	if err := run(cmd, []string{"proj"}, d.RuntimeCLI); err != nil {
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
	}
	err := run(cmd, []string{"proj"}, d.RuntimeCLI)
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
	}

	done := make(chan error, 1)
	var got Results[string]
	go func() {
		res, err := cmd.Run([]string{"proj"}, d.RuntimeCLI)
		if err == nil {
			got = res
			err = Lines(res, d.RuntimeCLI)
		}
		done <- err
	}()
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

// TestRunSingleRepository covers the single-repository dispatch: only the named
// repository runs, the tree the run reports is the project narrowed to it,
// and its warning shows on its line without failing the run.
func TestRunSingleRepository(t *testing.T) {
	t.Parallel()

	d := deps(t, 1, clitest.Cloned("api"), clitest.Cloned("web"))

	var reached []string
	cmd := Command[string]{
		Verb: "testing",
		Body: func(_ context.Context, repo Repo, _ types.RuntimeCLI) (string, error) {
			reached = append(reached, repo.GetName())
			return lineFor(repo.GetName()), types.NewWarning("skip me")
		},
	}

	res, err := cmd.Run([]string{"proj", "api"}, d.RuntimeCLI)
	if err != nil {
		t.Fatalf("Run error = %v, want nil", err)
	}
	if err := Lines(res, d.RuntimeCLI); err != nil {
		t.Fatalf("Lines error = %v, want a warning to leave the exit code alone", err)
	}
	if fmt.Sprint(reached) != fmt.Sprint([]string{"api"}) {
		t.Errorf("body reached %v, want only the named repository", reached)
	}
	if len(res.Project.Repos) != 1 || res.Project.Repos[0].Name != "api" || len(res.Results) != 1 {
		t.Errorf("Results = %+v, want the project narrowed to the named repository", res)
	}
	got := d.Result()
	if !strings.Contains(got, "api") || !strings.Contains(got, "skip me") {
		t.Errorf("Result Output = %q, want the named repository's line", got)
	}
	if strings.Contains(got, "proj") {
		t.Errorf("Result Output = %q, want no project title", got)
	}
}

// TestRunSingleRepositoryFailure: a named repository's failure goes through
// the same epilogue as a whole run, so the exit code and the diagnostic are
// decided in one place.
func TestRunSingleRepositoryFailure(t *testing.T) {
	t.Parallel()

	d := deps(t, 1, clitest.Cloned("api"))
	cmd := Command[string]{
		Verb: "testing",
		Body: func(context.Context, Repo, types.RuntimeCLI) (string, error) {
			return "", errors.New("boom")
		},
	}
	err := run(cmd, []string{"proj", "api"}, d.RuntimeCLI)
	if err == nil || !strings.Contains(err.Error(), "completed with errors") {
		t.Fatalf("error = %v, want the run reported as failed", err)
	}
	if got := d.Diagnostic(); !strings.Contains(got, "1 error:") || !strings.Contains(got, "boom") {
		t.Errorf("Diagnostic Output = %q, want the epilogue", got)
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
	}
	err := run(cmd, []string{"proj"}, d.RuntimeCLI)
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
	}
	if err := run(cmd, []string{"proj"}, d.RuntimeCLI); err == nil {
		t.Fatal("Run error = nil, want the undeclared state to fail the run")
	}
	if fmt.Sprint(reached) != fmt.Sprint([]string{"gone"}) {
		t.Errorf("body reached %v, want the declared state only", reached)
	}
}

// TestRunKeepsBodyErrorsBare: a body's error is kept exactly as returned, so
// the repository's line shows the bare reason next to its title; the epilogue
// list is where the repository's name and path are attached, and a warning
// is left as it is on both.
func TestRunKeepsBodyErrorsBare(t *testing.T) {
	t.Parallel()

	d := deps(t, 1, clitest.Cloned("api"))

	boom := errors.New("boom")
	plain := Command[string]{
		Verb: "testing",
		Body: func(context.Context, Repo, types.RuntimeCLI) (string, error) { return "", boom },
	}
	res, err := plain.Run([]string{"proj", "api"}, d.RuntimeCLI)
	if err != nil {
		t.Fatalf("Run error = %v, want nil", err)
	}
	if got := res.Results[0].Err; got != boom { //nolint:errorlint // identity is the assertion
		t.Fatalf("body error = %v, want it kept as returned", got)
	}
	errs := res.Errors()
	var wrapped *types.Warning
	if len(errs) != 1 || !errors.As(errs[0], &wrapped) {
		t.Fatalf("Errors() = %v, want the failure wrapped for the epilogue", errs)
	}
	if wrapped.Title != "api" || wrapped.Type != types.ErrorType || !errors.Is(errs[0], boom) {
		t.Errorf("wrapped error = %+v, want the repository named and a real failure", wrapped)
	}

	warning := types.NewWarning("already handled")
	downgraded := Command[string]{
		Verb: "testing",
		Body: func(context.Context, Repo, types.RuntimeCLI) (string, error) { return "", warning },
	}
	res, err = downgraded.Run([]string{"proj", "api"}, d.RuntimeCLI)
	if err != nil {
		t.Fatalf("Run error = %v, want nil", err)
	}
	if errs := res.Errors(); len(errs) != 1 || errs[0] != warning { //nolint:errorlint // identity is the assertion
		t.Errorf("Errors() = %v, want the warning left untouched", errs)
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
			}
			if err := run(cmd, args, d.RuntimeCLI); err != nil {
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
			}
			if err := run(cmd, tc.args, d.RuntimeCLI); err != nil {
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
	}
	res, err := cmd.Run([]string{"proj"}, d.RuntimeCLI)
	if err != nil {
		t.Fatalf("Run error = %v, want nil", err)
	}
	for _, r := range res.Results {
		got = append(got, r.Value)
	}
	if len(got) != 2 || got[0].name != "api" || got[1].name != "web" {
		t.Fatalf("collected = %+v, want both probes in tree order", got)
	}
}

// TestRunHandsDisplayPaths: every body receives its repository's display path,
// and Lines pads the titles beneath one project to the widest of them so the
// bodies align.
func TestRunHandsDisplayPaths(t *testing.T) {
	t.Parallel()

	d := deps(t, 1, clitest.Cloned("api"), clitest.Cloned("much-longer-name"))

	var paths []string
	cmd := Command[string]{
		Verb: "testing",
		Body: func(_ context.Context, repo Repo, _ types.RuntimeCLI) (string, error) {
			paths = append(paths, repo.Path)
			return lineFor(repo.GetName()), nil
		},
	}
	if err := run(cmd, []string{"proj"}, d.RuntimeCLI); err != nil {
		t.Fatalf("Run error = %v, want nil", err)
	}
	if want := []string{"api", "much-longer-name"}; fmt.Sprint(paths) != fmt.Sprint(want) {
		t.Fatalf("paths = %v, want %v", paths, want)
	}
	lines := strings.Split(strings.TrimRight(d.Result(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("Result Output = %q, want one line per repository", d.Result())
	}
	if a, b := strings.Index(lines[0], "LINE:"), strings.Index(lines[1], "LINE:"); a != b {
		t.Errorf("bodies start at columns %d and %d, want them aligned:\n%s", a, b, d.Result())
	}
}
