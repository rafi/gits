package checkout

import (
	"context"
	"errors"
	"strings"
	"testing"

	"charm.land/huh/v2"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cli/clitest"
	"github.com/rafi/gits/internal/git"
	"github.com/rafi/gits/internal/types"
)

// The branch prompt runs a huh form on the terminal, and `checkout` reaches it
// even when both arguments are given — there is no argument that skips it, the
// way an explicit repository name skips the interactive finder. So the package
// has one interactive seam, runBranchPrompt, which stubPrompt below replaces.
// Everything around it is then driven through the real entry point: the state
// guard, the three outcome lines a repository gets once its prompt returns,
// the project traversal, the sub-project separator and the error epilogue.
//
// What is deliberately not covered is the form itself — the keys huh binds and
// what it draws — which belongs to huh's own tests.

var (
	errBoom = errors.New("boom")
)

// fakeBranchClient implements the two methods promptRepo reaches before the
// (TTY) prompt; everything else is inherited from clitest.FakeGit and panics
// if reached. CurrentBranch succeeds so control flows into AllBranches, which
// returns whatever the test asked for.
type fakeBranchClient struct {
	clitest.FakeGit

	branches []string
	err      error
}

func (fakeBranchClient) CurrentBranch(context.Context, string) (string, error) {
	return "main", nil
}

func (c fakeBranchClient) AllBranches(context.Context, string) ([]string, error) {
	return c.branches, c.err
}

// compile-time check: fakeBranchClient must satisfy the git client interface.
var _ git.Client = fakeBranchClient{}

// TestExecCheckoutAbortsOnNonOKState covers `gits checkout acme bad`: the
// state guard runs before the prompt, and the Repository's own Reason reaches
// Diagnostic Output on the line its title opens.
func TestExecCheckoutAbortsOnNonOKState(t *testing.T) {
	t.Parallel()

	deps := clitest.New(t, clitest.FakeGit{}).WithProject("acme", clitest.Broken("bad"))

	err := ExecCheckout([]string{"acme", "bad"}, deps.RuntimeCLI)
	if err == nil {
		t.Fatal("ExecCheckout error = nil, want the state to abort the command")
	}

	// The title and the message share one line, terminated once: the abort
	// helper ends the line the title opened.
	got := deps.Diagnostic()
	if !strings.Contains(got, "bad") || !strings.HasSuffix(got, clitest.BrokenReason+"\n") {
		t.Errorf("Diagnostic Output = %q, want the repository title then its Reason", got)
	}
	if n := strings.Count(got, "\n"); n != 1 {
		t.Errorf("Diagnostic Output = %q, want one terminated line, got %d", got, n)
	}
	if got := deps.Result(); got != "" {
		t.Errorf("Result Output = %q, want empty — nothing was checked out", got)
	}
}

// TestExecCheckoutProjectSkipsNonOKRepositories covers `gits checkout acme`:
// the whole-Project path titles the project on Result Output, passes over
// every Repository that is not `ok` without prompting, and counts both in the
// error epilogue on Diagnostic Output.
func TestExecCheckoutProjectSkipsNonOKRepositories(t *testing.T) {
	t.Parallel()

	deps := clitest.New(t, clitest.FakeGit{}).
		WithProject("acme", clitest.NotCloned("gone"), clitest.Broken("bad"))

	err := ExecCheckout([]string{"acme"}, deps.RuntimeCLI)
	if err == nil {
		t.Fatal("ExecCheckout error = nil, want the skipped repositories to fail the run")
	}
	if !strings.Contains(err.Error(), "completed with errors") {
		t.Errorf("ExecCheckout error = %v, want the run reported as failed", err)
	}

	if got := deps.Result(); !strings.Contains(got, "acme") {
		t.Errorf("Result Output = %q, want the project title", got)
	}
	got := deps.Diagnostic()
	for _, want := range []string{"not cloned", clitest.BrokenReason, "2 errors:"} {
		if !strings.Contains(got, want) {
			t.Errorf("Diagnostic Output = %q, want it to contain %q", got, want)
		}
	}
}

// TestExecCheckoutTitlesEverySubProject covers the recursion: every project
// node the recursion descends into titles itself on Result Output, separated
// from the one above by a blank line, so a repository's line is always under the
// project it belongs to.
func TestExecCheckoutTitlesEverySubProject(t *testing.T) {
	t.Parallel()

	deps := clitest.New(t, clitest.FakeGit{}).WithProject("acme", clitest.NotCloned("gone"))

	project := deps.Projects["acme"]
	project.SubProjects = []domain.Project{{
		Name:  "tools",
		Path:  project.Path,
		Repos: []domain.Repository{{Name: "cli", Dir: "cli", Src: clitest.RepoSrc("cli")}},
	}}
	deps.Projects["acme"] = project

	if err := ExecCheckout([]string{"acme"}, deps.RuntimeCLI); err == nil {
		t.Fatal("ExecCheckout error = nil, want the skipped repositories to fail the run")
	}

	if want := ":: acme\n\n:: tools\n"; deps.Result() != want {
		t.Errorf("Result Output = %q, want exactly %q", deps.Result(), want)
	}
}

// TestPromptRepoBranchesErrorDoesNotExit proves T3 (#4): a Branches failure
// surfaces as a returned error instead of aborting the whole process. The
// original defect called [log.Fatal], which exits; if it ever returns here,
// the test binary dies and this test fails loudly by not reporting at all.
func TestPromptRepoBranchesErrorDoesNotExit(t *testing.T) {
	t.Parallel()

	deps := clitest.New(t, fakeBranchClient{err: errBoom})

	got, _, err := promptRepo("title", "/path", deps.RuntimeCLI)
	if err == nil {
		t.Fatalf("promptRepo with failing Branches = (%q, nil), want a wrapped error", got)
	}
	if !errors.Is(err, errBoom) {
		t.Fatalf("error %v does not wrap the Branches failure", err)
	}
}

// TestPromptRepoNoBranchesErrors proves AC-4: an empty branch list returns an
// error instead of opening a prompt. huh's select refuses to submit while it
// has no options, so prompting would strand the user in a form only Ctrl-C
// escapes. Defensive rather than reachable today — a repository with no
// commits fails a step earlier, in CurrentBranch.
func TestPromptRepoNoBranchesErrors(t *testing.T) {
	t.Parallel()

	deps := clitest.New(t, fakeBranchClient{branches: []string{}})

	got, current, err := promptRepo("title", "/path", deps.RuntimeCLI)
	if err == nil {
		t.Fatalf("promptRepo with no branches = (%q, %q, nil), want an error", got, current)
	}
}

// TestNewBranchPromptPreselectsCurrent proves AC-5: the prompt opens on the
// branch that is currently checked out, not on the first one listed. Hovered
// reports where the cursor sits, which is the claim; GetValue would only
// report the binding the caller seeded.
func TestNewBranchPromptPreselectsCurrent(t *testing.T) {
	t.Parallel()

	const current = "develop"

	want := current
	prompt := newBranchPrompt("title", []string{"main", current, "next"}, &want)

	got, ok := prompt.Hovered()
	if !ok {
		t.Fatal("prompt opens with no option under the cursor")
	}
	if got != current {
		t.Fatalf("cursor opens on %q, want the current branch %q", got, current)
	}
}

// stubPrompt replaces the package's one interactive seam for the duration of a
// test, so the outcome lines a repository gets after its prompt returns are
// reachable without a terminal. choose is given the branch currently checked
// out and returns the branch the user picked.
func stubPrompt(t *testing.T, choose func(current string) (string, error)) {
	t.Helper()
	original := runBranchPrompt
	t.Cleanup(func() { runBranchPrompt = original })

	runBranchPrompt = func(_ *huh.Select[string], choice *string) error {
		want, err := choose(*choice)
		if err != nil {
			return err
		}
		*choice = want
		return nil
	}
}

// checkoutGit answers the three methods a completed checkout reaches, and
// records what Checkout was asked to do.
type checkoutGit struct {
	clitest.FakeGit

	current     string
	branches    []string
	err         error
	checkedOut  string
	checkoutErr error
}

func (c *checkoutGit) CurrentBranch(context.Context, string) (string, error) {
	return c.current, c.err
}

func (c *checkoutGit) AllBranches(context.Context, string) ([]string, error) {
	return c.branches, nil
}

func (c *checkoutGit) Checkout(_ context.Context, _, branch string) error {
	c.checkedOut = branch
	return c.checkoutErr
}

// TestCheckoutRepoSwitchesBranch covers the success line: a pick that differs
// from the current branch reaches git and is reported on Result Output.
//
//nolint:paralleltest // stubPrompt replaces a package-level seam.
func TestCheckoutRepoSwitchesBranch(t *testing.T) {
	stubPrompt(t, func(string) (string, error) { return "feat", nil })

	gitClient := &checkoutGit{current: "main", branches: []string{"main", "feat"}}
	deps := clitest.New(t, gitClient).WithProject("acme", clitest.Cloned("api"))

	if err := ExecCheckout([]string{"acme", "api"}, deps.RuntimeCLI); err != nil {
		t.Fatalf("ExecCheckout: %v", err)
	}
	if gitClient.checkedOut != "feat" {
		t.Errorf("Checkout branch = %q, want %q", gitClient.checkedOut, "feat")
	}
	got := deps.Result()
	for _, want := range []string{"api", `Switched to branch "feat"`} {
		if !strings.Contains(got, want) {
			t.Errorf("Result Output = %q, want it to contain %q", got, want)
		}
	}
}

// TestCheckoutRepoKeepsCurrentBranch covers the no-op line: picking the branch
// already checked out reports it and never calls git.
//
//nolint:paralleltest // stubPrompt replaces a package-level seam.
func TestCheckoutRepoKeepsCurrentBranch(t *testing.T) {
	stubPrompt(t, func(current string) (string, error) { return current, nil })

	gitClient := &checkoutGit{current: "main", branches: []string{"main", "feat"}}
	deps := clitest.New(t, gitClient).WithProject("acme", clitest.Cloned("api"))

	if err := ExecCheckout([]string{"acme", "api"}, deps.RuntimeCLI); err != nil {
		t.Fatalf("ExecCheckout: %v", err)
	}
	if gitClient.checkedOut != "" {
		t.Errorf("Checkout branch = %q, want no checkout for an unchanged branch", gitClient.checkedOut)
	}
	if got := deps.Result(); !strings.Contains(got, "main") {
		t.Errorf("Result Output = %q, want the current branch reported", got)
	}
}

// TestCheckoutRepoReportsFailure covers the failure line: git's error is shown
// against the repository and fails the command.
//
//nolint:paralleltest // stubPrompt replaces a package-level seam.
func TestCheckoutRepoReportsFailure(t *testing.T) {
	stubPrompt(t, func(string) (string, error) { return "feat", nil })

	gitClient := &checkoutGit{
		current: "main", branches: []string{"main", "feat"}, checkoutErr: errBoom,
	}
	deps := clitest.New(t, gitClient).WithProject("acme", clitest.Cloned("api"))

	err := ExecCheckout([]string{"acme", "api"}, deps.RuntimeCLI)
	if err == nil {
		t.Fatal("ExecCheckout error = nil, want the checkout failure")
	}
	if !strings.Contains(err.Error(), "api") {
		t.Errorf("ExecCheckout error = %v, want it to name the repository", err)
	}
	if got := deps.Result(); !strings.Contains(got, errBoom.Error()) {
		t.Errorf("Result Output = %q, want git's error", got)
	}
}

// TestCheckoutRepoAbortIsAWarning covers Ctrl-C at the prompt: it is a
// documented pass-over, not a failure, so it comes back as a warning.
//
//nolint:paralleltest // stubPrompt replaces a package-level seam.
func TestCheckoutRepoAbortIsAWarning(t *testing.T) {
	stubPrompt(t, func(string) (string, error) { return "", huh.ErrUserAborted })

	gitClient := &checkoutGit{current: "main", branches: []string{"main", "feat"}}
	deps := clitest.New(t, gitClient).WithProject("acme", clitest.Cloned("api"))

	err := ExecCheckout([]string{"acme", "api"}, deps.RuntimeCLI)
	if err == nil {
		t.Fatal("ExecCheckout error = nil, want the abort reported")
	}
	if !types.IsWarning(err) {
		t.Errorf("ExecCheckout error = %T (%v), want a downgradeable warning", err, err)
	}
}

// TestCheckoutProjectAbortStopsTraversal covers the whole-project path: an
// abort stops the walk rather than prompting for every remaining repository,
// and is a warning, so the run itself still succeeds.
//
//nolint:paralleltest // stubPrompt replaces a package-level seam.
func TestCheckoutProjectAbortStopsTraversal(t *testing.T) {
	prompts := 0
	stubPrompt(t, func(string) (string, error) {
		prompts++
		return "", huh.ErrUserAborted
	})

	gitClient := &checkoutGit{current: "main", branches: []string{"main", "feat"}}
	deps := clitest.New(t, gitClient).
		WithProject("acme", clitest.Cloned("api"), clitest.Cloned("web"))

	if err := ExecCheckout([]string{"acme"}, deps.RuntimeCLI); err != nil {
		t.Fatalf("ExecCheckout: %v — an abort is a warning, not a failed run", err)
	}
	if prompts != 1 {
		t.Errorf("prompts = %d, want 1 — an abort stops the traversal", prompts)
	}
}

// TestPromptRepoCurrentBranchErrorDoesNotExit covers the other pre-prompt
// failure: a repository whose current branch cannot be read fails alone.
//
//nolint:paralleltest // stubPrompt replaces a package-level seam.
func TestPromptRepoCurrentBranchErrorDoesNotExit(t *testing.T) {
	gitClient := &checkoutGit{err: errBoom}
	deps := clitest.New(t, gitClient).WithProject("acme", clitest.Cloned("api"))

	err := ExecCheckout([]string{"acme", "api"}, deps.RuntimeCLI)
	if err == nil {
		t.Fatal("ExecCheckout error = nil, want the branch lookup failure")
	}
	if !strings.Contains(err.Error(), "unable to get branch") {
		t.Errorf("ExecCheckout error = %v, want it to name the branch lookup", err)
	}
}
