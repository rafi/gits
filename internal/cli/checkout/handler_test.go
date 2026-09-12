package checkout

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	log "github.com/sirupsen/logrus"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cli/clitest"
	"github.com/rafi/gits/internal/git"
)

// The branch prompt itself has no test, and that is deliberate rather than an
// oversight: promptRepo runs a huh form on the terminal, and `checkout`
// reaches it even when both arguments are given — there is no argument that
// skips it, the way an explicit repository name skips the interactive finder.
// Covering it needs a seam for interactive selection, which does not exist
// yet.
//
// So the entry-point tests below drive ExecCheckout with repositories that
// abort at the state guard, ahead of the prompt. That reaches the guard, the
// project traversal, the sub-project separator and the error epilogue; the
// three outcome lines a repository gets after its prompt returns stay uncovered
// until that seam exists.

var (
	errBoom     = errors.New("boom")
	errFakeExit = errors.New("fake exit")
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
// surfaces as a returned error instead of aborting the whole process via
// [log.Fatal] / [os.Exit]. logrus's exit is neutralized so the buggy path (if
// present) records the exit and panics before the TTY prompt, rather than
// killing the test binary.
func TestPromptRepoBranchesErrorDoesNotExit(t *testing.T) {
	t.Parallel()

	std := log.StandardLogger()
	origExit, origOut := std.ExitFunc, std.Out
	exited := false
	std.ExitFunc = func(int) { exited = true; panic(errFakeExit) }
	std.SetOutput(io.Discard) // swallow the Fatal log line
	t.Cleanup(func() { std.ExitFunc = origExit; std.SetOutput(origOut) })

	deps := clitest.New(t, fakeBranchClient{err: errBoom})

	var (
		got string
		err error
	)
	func() {
		defer func() {
			if r := recover(); r != nil {
				if err, ok := r.(error); !ok || !errors.Is(err, errFakeExit) {
					panic(r)
				}
			}
		}()
		got, _, err = promptRepo("title", "/path", deps.RuntimeCLI)
	}()

	if exited {
		t.Fatal("promptRepo called log.Fatal (os.Exit) on Branches failure; want a returned error")
	}
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
