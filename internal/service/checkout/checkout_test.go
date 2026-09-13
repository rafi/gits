package checkout

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/infra/git"
	"github.com/rafi/gits/internal/service"
)

// The service is the half of `gits checkout` that a test can reach without a
// terminal: listing branches and switching between them. The prompt around it
// lives in the view, and huh's own tests cover the form.

var errBoom = errors.New("boom")

// fakeGit answers the three methods the two functions reach and records the
// checkout it was asked for. Every other method is inherited unimplemented
// and panics if reached, which is the assertion that neither function does
// more git work than it claims to.
type fakeGit struct {
	git.Client

	current     string
	currentErr  error
	branches    []string
	branchesErr error

	checkedOut  string
	checkoutErr error
}

func (c *fakeGit) CurrentBranch(context.Context, string) (string, error) {
	return c.current, c.currentErr
}

func (c *fakeGit) AllBranches(context.Context, string) ([]string, error) {
	return c.branches, c.branchesErr
}

func (c *fakeGit) Checkout(_ context.Context, _, branch string) error {
	c.checkedOut = branch
	return c.checkoutErr
}

func runtime(t *testing.T, gitClient git.Client) service.Runtime {
	t.Helper()
	return service.Runtime{Ctx: t.Context(), Git: gitClient}
}

var repo = domain.Repository{Name: "api", AbsPath: "/src/api"}

// TestBranchesListsAndReportsCurrent is the happy path: both git calls answer,
// and the branch checked out comes back beside the list it belongs to.
func TestBranchesListsAndReportsCurrent(t *testing.T) {
	t.Parallel()

	gitClient := &fakeGit{current: "main", branches: []string{"main", "feat"}}

	branches, current, err := Branches(t.Context(), repo, runtime(t, gitClient))
	if err != nil {
		t.Fatalf("Branches: %v", err)
	}
	if current != "main" {
		t.Errorf("current = %q, want %q", current, "main")
	}
	if len(branches) != 2 || branches[0] != "main" {
		t.Errorf("branches = %v, want the client's list", branches)
	}
}

// TestBranchesErrors covers the three ways listing fails. Each returns rather
// than exiting: a repository that cannot be read fails alone, leaving the rest
// of a traversal to run.
func TestBranchesErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		client *fakeGit
		want   string
	}{
		{
			name:   "current branch unreadable",
			client: &fakeGit{currentErr: errBoom},
			want:   "unable to get branch",
		},
		{
			name:   "branch list unreadable",
			client: &fakeGit{current: "main", branchesErr: errBoom},
			want:   "unable to read branches",
		},
		{
			// An option-less prompt cannot be submitted, only aborted, so an
			// empty list must never reach one.
			name:   "no branches at all",
			client: &fakeGit{current: "main"},
			want:   "no branches",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, _, err := Branches(t.Context(), repo, runtime(t, tt.client))
			if err == nil {
				t.Fatalf("Branches error = nil, want %q", tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Branches error = %v, want it to mention %q", err, tt.want)
			}
		})
	}
}

// TestBranchesWrapsClientError proves the wrapping keeps the cause matchable,
// so a caller can still tell a git failure from a validation one.
func TestBranchesWrapsClientError(t *testing.T) {
	t.Parallel()

	_, _, err := Branches(t.Context(), repo, runtime(t, &fakeGit{current: "main", branchesErr: errBoom}))
	if !errors.Is(err, errBoom) {
		t.Errorf("error %v does not wrap the client failure", err)
	}
}

// TestSwitchChecksOut is the happy path: a different branch reaches git.
func TestSwitchChecksOut(t *testing.T) {
	t.Parallel()

	gitClient := &fakeGit{current: "main"}

	if err := Switch(t.Context(), repo, "feat", runtime(t, gitClient)); err != nil {
		t.Fatalf("Switch: %v", err)
	}
	if gitClient.checkedOut != "feat" {
		t.Errorf("Checkout branch = %q, want %q", gitClient.checkedOut, "feat")
	}
}

// TestSwitchIsANoOpForTheCurrentBranch proves the guard: asking for the branch
// already checked out touches nothing, so a caller that did not compare cannot
// produce a spurious checkout.
func TestSwitchIsANoOpForTheCurrentBranch(t *testing.T) {
	t.Parallel()

	gitClient := &fakeGit{current: "main"}

	if err := Switch(t.Context(), repo, "main", runtime(t, gitClient)); err != nil {
		t.Fatalf("Switch: %v", err)
	}
	if gitClient.checkedOut != "" {
		t.Errorf("Checkout branch = %q, want no checkout at all", gitClient.checkedOut)
	}
}

// TestSwitchReportsGitFailure proves git's refusal is returned unwrapped, so
// the view can print it as the reason the line failed.
func TestSwitchReportsGitFailure(t *testing.T) {
	t.Parallel()

	gitClient := &fakeGit{current: "main", checkoutErr: errBoom}

	err := Switch(t.Context(), repo, "feat", runtime(t, gitClient))
	if !errors.Is(err, errBoom) {
		t.Errorf("Switch error = %v, want git's failure", err)
	}
}

// TestSwitchReportsUnreadableCurrentBranch covers the guard's own failure: it
// cannot tell a no-op from a switch, so it refuses rather than guessing.
func TestSwitchReportsUnreadableCurrentBranch(t *testing.T) {
	t.Parallel()

	gitClient := &fakeGit{currentErr: errBoom}

	err := Switch(t.Context(), repo, "feat", runtime(t, gitClient))
	if err == nil || !errors.Is(err, errBoom) {
		t.Fatalf("Switch error = %v, want the branch lookup failure", err)
	}
	if gitClient.checkedOut != "" {
		t.Errorf("Checkout branch = %q, want nothing checked out", gitClient.checkedOut)
	}
}
