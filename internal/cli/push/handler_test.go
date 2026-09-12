package push

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/rafi/gits/internal/cli/clitest"
	"github.com/rafi/gits/internal/git"
)

// Every test here drives ExecPush — the command's real entry point — with
// explicit arguments, so argument parsing, the choice between walking a whole
// Project and acting on a single Repository, the error epilogue and the exit
// code all run for real, and the interactive finder is never reached.

// pushCall records one push: the repository it went to, and the destination and
// options gits named. The destination is a requirement of ADR-0002, not an
// implementation detail, so it is asserted on rather than inferred from output.
type pushCall struct {
	Repo   string
	Target git.PushTarget
	Opts   git.PushOptions
}

// fakeGit implements the two calls pushRepo makes. Everything else is
// inherited from clitest.FakeGit and panics if reached, except IsRepo, which is
// overridden to record that classification ran at all.
type fakeGit struct {
	clitest.FakeGit

	head    git.HeadRef
	headErr error
	pushOut string
	pushErr error

	mu         sync.Mutex
	pushes     []pushCall
	classified bool
}

func (f *fakeGit) IsRepo(context.Context, string) bool {
	f.mu.Lock()
	f.classified = true
	f.mu.Unlock()
	return true
}

func (f *fakeGit) HeadUpstream(context.Context, string) (git.HeadRef, error) {
	return f.head, f.headErr
}

func (f *fakeGit) Push(
	_ context.Context, path string, target git.PushTarget, opts git.PushOptions,
) (string, error) {
	f.mu.Lock()
	f.pushes = append(f.pushes, pushCall{filepath.Base(path), target, opts})
	f.mu.Unlock()
	return f.pushOut, f.pushErr
}

// Pushes returns every push, in the order the Traversal reached them.
func (f *fakeGit) Pushes() []pushCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.pushes)
}

// Classified reports whether any repository was classified, which is how a test
// tells that nothing was loaded.
func (f *fakeGit) Classified() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.classified
}

// pushed returns the name of every repository pushed.
func pushed(calls []pushCall) []string {
	names := make([]string, 0, len(calls))
	for _, c := range calls {
		names = append(names, c.Repo)
	}
	return names
}

// tracking is a fake whose repositories all sit on a branch with an Upstream
// that resolves, which is the condition push works under.
func tracking() *fakeGit {
	return &fakeGit{
		head:    git.HeadRef{Branch: "main", Upstream: "origin/main"},
		pushOut: "Everything up-to-date",
	}
}

// TestExecPushProject covers `gits push acme`: every repository of the project
// is pushed to the destination derived from its Upstream, and its result
// reaches Result Output under the project title.
//
// Diagnostic Output being empty is the second assertion: a run with nothing to
// report emits no epilogue, and the live progress reporter — handed a buffer
// rather than a terminal — emits nothing at all, so no ANSI can reach any
// assertion in this package.
func TestExecPushProject(t *testing.T) {
	t.Parallel()

	g := tracking()
	deps := clitest.New(t, g).WithProject("acme", clitest.Cloned("api"), clitest.Cloned("web"))

	if err := ExecPush(git.PushOptions{}, []string{"acme"}, deps.RuntimeCLI); err != nil {
		t.Fatalf("ExecPush error = %v, want nil", err)
	}

	got := deps.Result()
	for _, want := range []string{"api", "web", "main -> origin/main", "up-to-date"} {
		if !strings.Contains(got, want) {
			t.Errorf("Result Output = %q, want it to contain %q", got, want)
		}
	}
	want := []pushCall{
		{"api", git.PushTarget{Remote: "origin", Refspec: "main:main"}, git.PushOptions{}},
		{"web", git.PushTarget{Remote: "origin", Refspec: "main:main"}, git.PushOptions{}},
	}
	if !slices.Equal(g.Pushes(), want) {
		t.Errorf("pushes = %+v, want %+v", g.Pushes(), want)
	}
	if got := deps.Diagnostic(); got != "" {
		t.Errorf("Diagnostic Output = %q, want empty", got)
	}
}

// TestExecPushSingleRepo covers `gits push acme api`: the second argument
// selects one repository, and only that one is pushed and rendered — without
// the project title the whole-project path prints.
func TestExecPushSingleRepo(t *testing.T) {
	t.Parallel()

	g := tracking()
	deps := clitest.New(t, g).WithProject("acme", clitest.Cloned("api"), clitest.Cloned("web"))

	if err := ExecPush(git.PushOptions{}, []string{"acme", "api"}, deps.RuntimeCLI); err != nil {
		t.Fatalf("ExecPush error = %v, want nil", err)
	}

	got := deps.Result()
	if !strings.Contains(got, "api") || !strings.Contains(got, "up-to-date") {
		t.Errorf("Result Output = %q, want the selected repository's result", got)
	}
	if strings.Contains(got, "web") {
		t.Errorf("Result Output = %q, want nothing about the other repository", got)
	}
	if want := []string{"api"}; !slices.Equal(pushed(g.Pushes()), want) {
		t.Errorf("pushed %v, want %v", pushed(g.Pushes()), want)
	}
}

// TestExecPushSkipsNonOKRepositories covers the state guard: a repository that
// is not cloned, and one whose configuration is defective, are both passed over
// without a push and both count toward the exit code. The defective one reports
// the Reason it was classified with, not a generic message.
func TestExecPushSkipsNonOKRepositories(t *testing.T) {
	t.Parallel()

	g := tracking()
	deps := clitest.New(t, g).WithProject("acme",
		clitest.Cloned("api"), clitest.NotCloned("gone"), clitest.Broken("bad"))

	err := ExecPush(git.PushOptions{}, []string{"acme"}, deps.RuntimeCLI)
	if err == nil {
		t.Fatal("ExecPush error = nil, want the skipped repositories to fail the run")
	}

	if want := []string{"api"}; !slices.Equal(pushed(g.Pushes()), want) {
		t.Errorf("pushed %v, want only the cloned repository %v", pushed(g.Pushes()), want)
	}
	for _, want := range []string{"not cloned", clitest.BrokenReason} {
		if got := deps.Result(); !strings.Contains(got, want) {
			t.Errorf("Result Output = %q, want it to contain %q", got, want)
		}
		if got := deps.Diagnostic(); !strings.Contains(got, want) {
			t.Errorf("Diagnostic Output = %q, want the epilogue to contain %q", got, want)
		}
	}
	if got := deps.Diagnostic(); !strings.Contains(got, "2 errors:") {
		t.Errorf("Diagnostic Output = %q, want both skips counted in the epilogue", got)
	}
}

// TestExecPushFailureReportsEpilogue covers git refusing a push: its message
// reaches Result Output on the repository's line and Diagnostic Output in the
// error epilogue, and the run reports failure.
func TestExecPushFailureReportsEpilogue(t *testing.T) {
	t.Parallel()

	g := tracking()
	g.pushErr = errors.New("failed to push some refs: non-fast-forward")
	deps := clitest.New(t, g).WithProject("acme", clitest.Cloned("api"))

	err := ExecPush(git.PushOptions{}, []string{"acme"}, deps.RuntimeCLI)
	if err == nil {
		t.Fatal("ExecPush error = nil, want the rejected push to fail the run")
	}
	if !strings.Contains(err.Error(), "completed with errors") {
		t.Errorf("ExecPush error = %v, want the run reported as failed", err)
	}
	if got := deps.Result(); !strings.Contains(got, "non-fast-forward") {
		t.Errorf("Result Output = %q, want git's message on the repository line", got)
	}
	got := deps.Diagnostic()
	if !strings.Contains(got, "1 error:") || !strings.Contains(got, "non-fast-forward") {
		t.Errorf("Diagnostic Output = %q, want the error epilogue", got)
	}
}

// TestExecPushUnpushableIsSkipped covers the two skips ADR-0002 requires:
// pushing a branch with no Upstream is undefined, and pushing one whose
// Upstream is gone would re-create the branch someone deleted on the Remote.
// Both are passed over with a rendered line, nothing is pushed, and — unlike
// every other skip — the run still succeeds.
//
// That no push is issued is asserted directly rather than inferred from the
// line: a push in the gone state succeeds, so the output alone would not say
// whether the ref was scattered.
func TestExecPushUnpushableIsSkipped(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		head git.HeadRef
		want []string
	}{
		{
			name: "no upstream",
			head: git.HeadRef{Branch: "main"},
			want: []string{"skipped", git.ErrNoUpstream.Error()},
		},
		{
			name: "gone upstream",
			head: git.HeadRef{Branch: "feat-b", Upstream: "origin/feat-b", Gone: true},
			want: []string{"skipped", "origin/feat-b", git.ErrUpstreamGone.Error()},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			g := &fakeGit{head: tc.head}
			deps := clitest.New(t, g).WithProject("acme", clitest.Cloned("api"))

			if err := ExecPush(git.PushOptions{}, []string{"acme"}, deps.RuntimeCLI); err != nil {
				t.Fatalf("ExecPush error = %v, want a skipped repository not to fail the run", err)
			}
			if got := g.Pushes(); len(got) != 0 {
				t.Errorf("pushes = %+v, want none for a skipped repository", got)
			}
			got := deps.Result()
			for _, want := range tc.want {
				if !strings.Contains(got, want) {
					t.Errorf("Result Output = %q, want the line to contain %q", got, want)
				}
			}
			// Nothing git says about the condition reaches the user: the state
			// is read from a query that answers it, not from a failure.
			for _, banned := range []string{"fatal:", "@{upstream}", "ambiguous argument"} {
				if strings.Contains(got, banned) {
					t.Errorf("Result Output = %q, want no raw git error text (%q)", got, banned)
				}
			}
			if got := deps.Diagnostic(); got != "" {
				t.Errorf("Diagnostic Output = %q, want a warning to leave the epilogue empty", got)
			}
		})
	}
}

// TestExecPushSkipsReadDifferently covers the two skips being distinguishable:
// a branch merged and cleaned up on the Remote must not read like one that was
// never pushed.
func TestExecPushSkipsReadDifferently(t *testing.T) {
	t.Parallel()

	line := func(t *testing.T, head git.HeadRef) string {
		t.Helper()
		deps := clitest.New(t, &fakeGit{head: head}).
			WithProject("acme", clitest.Cloned("api"))
		if err := ExecPush(git.PushOptions{}, []string{"acme"}, deps.RuntimeCLI); err != nil {
			t.Fatalf("ExecPush error = %v, want nil", err)
		}
		return deps.Result()
	}

	none := line(t, git.HeadRef{Branch: "main"})
	gone := line(t, git.HeadRef{Branch: "feat-b", Upstream: "origin/feat-b", Gone: true})
	if none == gone {
		t.Errorf("both skips rendered %q, want the two conditions to read apart", none)
	}
	if strings.Contains(none, "origin/") {
		t.Errorf("Result Output = %q, want no Upstream named for a branch without one", none)
	}
}

// TestExecPushRenamedUpstream covers an Upstream under a different name than
// the local branch: the push goes to that name, not to the local branch's.
func TestExecPushRenamedUpstream(t *testing.T) {
	t.Parallel()

	g := &fakeGit{
		head:    git.HeadRef{Branch: "feature", Upstream: "upstream/release/v2"},
		pushOut: "ok",
	}
	deps := clitest.New(t, g).WithProject("acme", clitest.Cloned("api"))

	if err := ExecPush(git.PushOptions{}, []string{"acme", "api"}, deps.RuntimeCLI); err != nil {
		t.Fatalf("ExecPush error = %v, want nil", err)
	}

	want := []pushCall{{
		Repo:   "api",
		Target: git.PushTarget{Remote: "upstream", Refspec: "feature:release/v2"},
	}}
	if !slices.Equal(g.Pushes(), want) {
		t.Errorf("pushes = %+v, want %+v", g.Pushes(), want)
	}
}

// TestExecPushLocalUpstreamIsAnError covers a branch tracking another local
// branch: it has an Upstream that resolves, so neither skip applies, but there
// is nowhere to push it. The two conditions are independent — this one is
// unaffected by the Gone Upstream skip beside it.
func TestExecPushLocalUpstreamIsAnError(t *testing.T) {
	t.Parallel()

	g := &fakeGit{head: git.HeadRef{Branch: "feature", Upstream: "main"}}
	deps := clitest.New(t, g).WithProject("acme", clitest.Cloned("api"))

	err := ExecPush(git.PushOptions{}, []string{"acme", "api"}, deps.RuntimeCLI)
	if err == nil {
		t.Fatal("ExecPush error = nil, want an Upstream that names no remote to fail")
	}
	if got := deps.Diagnostic(); !strings.Contains(got, "1 error:") ||
		!strings.Contains(got, "not on a remote") {
		t.Errorf("Diagnostic Output = %q, want the epilogue to explain the Upstream", got)
	}
	if got := g.Pushes(); len(got) != 0 {
		t.Errorf("pushes = %+v, want none", got)
	}
}

// TestExecPushSelectsRefsSuspendsUpstream covers the ref-selecting flags: no
// branch is resolved, the destination is left to git, and a repository with no
// Upstream is pushed rather than skipped.
func TestExecPushSelectsRefsSuspendsUpstream(t *testing.T) {
	t.Parallel()

	for _, opts := range []git.PushOptions{
		{All: true}, {Branches: true}, {Tags: true},
	} {
		// A fake that would fail if the branch or the Upstream were consulted.
		g := &fakeGit{
			headErr: errors.New("HeadUpstream must not be called"),
			pushOut: "pushed",
		}
		deps := clitest.New(t, g).WithProject("acme", clitest.Cloned("api"))

		if err := ExecPush(opts, []string{"acme", "api"}, deps.RuntimeCLI); err != nil {
			t.Fatalf("ExecPush(%+v) error = %v, want nil", opts, err)
		}
		want := []pushCall{{Repo: "api", Opts: opts}}
		if !slices.Equal(g.Pushes(), want) {
			t.Errorf("ExecPush(%+v) pushes = %+v, want %+v — a zero target so git resolves it",
				opts, g.Pushes(), want)
		}
	}
}

// TestExecPushSelectsRefsFailure covers git refusing a ref-selecting push: the
// destination was left to git, so the failure is reported as itself and counts
// toward the exit code.
func TestExecPushSelectsRefsFailure(t *testing.T) {
	t.Parallel()

	g := &fakeGit{pushErr: errors.New("failed to push some refs")}
	deps := clitest.New(t, g).WithProject("acme", clitest.Cloned("api"))

	err := ExecPush(git.PushOptions{All: true}, []string{"acme", "api"}, deps.RuntimeCLI)
	if err == nil {
		t.Fatal("ExecPush error = nil, want the rejected push to fail")
	}
	if got := deps.Diagnostic(); !strings.Contains(got, "failed to push some refs") {
		t.Errorf("Diagnostic Output = %q, want git's message in the epilogue", got)
	}
}

// TestExecPushHeadFailure covers the Upstream lookup itself failing — an
// unreadable repository, or a cancellation: it is reported as itself on the
// line and in the epilogue and fails the run, rather than being downgraded to
// one of the documented skips. Nothing is pushed either way.
func TestExecPushHeadFailure(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{"canceled", context.Canceled, context.Canceled.Error()},
		{"git failure", errors.New("not a git repository"), "not a git repository"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			g := &fakeGit{headErr: tc.err}
			deps := clitest.New(t, g).WithProject("acme", clitest.Cloned("api"))

			err := ExecPush(git.PushOptions{}, []string{"acme", "api"}, deps.RuntimeCLI)
			if err == nil {
				t.Fatal("ExecPush error = nil, want the failed lookup to fail the run")
			}
			if got := deps.Diagnostic(); !strings.Contains(got, "1 error:") ||
				!strings.Contains(got, tc.want) {
				t.Errorf("Diagnostic Output = %q, want the epilogue to carry %q", got, tc.want)
			}
			if got := deps.Result(); !strings.Contains(got, tc.want) {
				t.Errorf("Result Output = %q, want the line to carry %q", got, tc.want)
			}
			if got := g.Pushes(); len(got) != 0 {
				t.Errorf("pushes = %+v, want none", got)
			}
		})
	}
}

// TestExecPushPassthroughFlagsReachGit covers the flags gits does not interpret
// arriving at the git client unchanged.
func TestExecPushPassthroughFlagsReachGit(t *testing.T) {
	t.Parallel()

	opts := git.PushOptions{FollowTags: true, Atomic: true, Prune: true, DryRun: true}
	g := tracking()
	deps := clitest.New(t, g).WithProject("acme", clitest.Cloned("api"))

	if err := ExecPush(opts, []string{"acme", "api"}, deps.RuntimeCLI); err != nil {
		t.Fatalf("ExecPush error = %v, want nil", err)
	}

	got := g.Pushes()
	if len(got) != 1 || got[0].Opts != opts {
		t.Errorf("pushes = %+v, want the options passed through as %+v", got, opts)
	}
}

// TestExecPushRejectsConflictingFlags covers a mutually exclusive flag
// combination being refused before anything is loaded: no repository is
// classified, so the run costs neither a provider round-trip nor a single
// remote. The same dependencies with a valid combination do reach
// classification, which is what makes that assertion meaningful.
func TestExecPushRejectsConflictingFlags(t *testing.T) {
	t.Parallel()

	g := tracking()
	deps := clitest.New(t, g).WithProject("acme", clitest.Cloned("api"))

	err := ExecPush(git.PushOptions{All: true, Tags: true}, []string{"acme"}, deps.RuntimeCLI)
	if err == nil {
		t.Fatal("ExecPush with --all and --tags = nil, want an error")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("ExecPush error = %v, want it to name the conflict", err)
	}
	if g.Classified() {
		t.Error("a repository was classified, want the flags rejected before anything is loaded")
	}
	if got := deps.Result(); got != "" {
		t.Errorf("Result Output = %q, want nothing rendered", got)
	}
	if got := deps.Diagnostic(); got != "" {
		t.Errorf("Diagnostic Output = %q, want nothing rendered", got)
	}

	if err := ExecPush(git.PushOptions{}, []string{"acme"}, deps.RuntimeCLI); err != nil {
		t.Fatalf("ExecPush error = %v, want nil", err)
	}
	if !g.Classified() {
		t.Error("the fixture was never classified, so the assertion above proves nothing")
	}
}
