package browse

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cli/clitest"
)

// The entry-point tests here drive all three of this package's commands with
// explicit arguments. ExecBrowse takes three — project, repository and branch —
// because a branch it was not given is selected interactively; the other two
// are preview sub-commands fzf invokes, and take their arguments the same way.

// previewGit is the fake behind every preview: one remote, one remote branch,
// and a commit log to render.
func previewGit() fakeBrowseGit {
	return fakeBrowseGit{
		remotes:     []string{"origin"},
		remoteRefs:  []string{"origin/main"},
		commitDates: []string{"2026-01-14"},
		commitLog:   "abc1234 initial commit",
		current:     "main",
	}
}

// TestExecBrowseWritesOverview covers `gits browse acme api main`: naming the
// branch is what keeps the run out of the interactive branch selection, and
// the overview it renders is Result Output.
func TestExecBrowseWritesOverview(t *testing.T) {
	t.Parallel()

	deps := clitest.New(t, previewGit()).WithProject("acme", clitest.Cloned("api"))

	if err := ExecBrowse([]string{"acme", "api", "main"}, deps.RuntimeCLI); err != nil {
		t.Fatalf("ExecBrowse error = %v, want nil", err)
	}

	got := deps.Result()
	for _, want := range []string{"api", "main", "origin", "abc1234 initial commit"} {
		if !strings.Contains(got, want) {
			t.Errorf("Result Output = %q, want it to contain %q", got, want)
		}
	}
	if got := deps.Diagnostic(); got != "" {
		t.Errorf("Diagnostic Output = %q, want empty", got)
	}
}

// TestExecBrowseAbortsOnNonOKState covers the state guard: a Repository that
// is not `ok` has no work tree to browse, so the command aborts with the
// Repository's own Reason on Diagnostic Output before any branch is selected.
func TestExecBrowseAbortsOnNonOKState(t *testing.T) {
	t.Parallel()

	deps := clitest.New(t, previewGit()).WithProject("acme", clitest.Broken("bad"))

	err := ExecBrowse([]string{"acme", "bad"}, deps.RuntimeCLI)
	if err == nil {
		t.Fatal("ExecBrowse error = nil, want the state to abort the command")
	}
	if want := clitest.BrokenReason + "\n"; deps.Diagnostic() != want {
		t.Errorf("Diagnostic Output = %q, want exactly %q", deps.Diagnostic(), want)
	}
	if got := deps.Result(); got != "" {
		t.Errorf("Result Output = %q, want empty", got)
	}
}

// TestExecBranchOverviewWritesOverview covers `gits branch-overview acme api
// main`, the preview fzf runs beside the branch list.
func TestExecBranchOverviewWritesOverview(t *testing.T) {
	t.Parallel()

	deps := clitest.New(t, previewGit()).WithProject("acme", clitest.Cloned("api"))

	if err := ExecBranchOverview([]string{"acme", "api", "main"}, deps.RuntimeCLI); err != nil {
		t.Fatalf("ExecBranchOverview error = %v, want nil", err)
	}

	got := deps.Result()
	for _, want := range []string{"api", "main", "abc1234 initial commit"} {
		if !strings.Contains(got, want) {
			t.Errorf("Result Output = %q, want it to contain %q", got, want)
		}
	}
	if got := deps.Diagnostic(); got != "" {
		t.Errorf("Diagnostic Output = %q, want empty", got)
	}
}

// TestExecBranchOverviewMissingArgs covers the two arguments the preview
// cannot do without, both refused before anything is loaded.
func TestExecBranchOverviewMissingArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want string
	}{
		{"no project", nil, "missing project name"},
		{"no repo", []string{"acme"}, "missing repo name"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			deps := clitest.New(t, previewGit()).WithProject("acme", clitest.Cloned("api"))

			err := ExecBranchOverview(tt.args, deps.RuntimeCLI)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ExecBranchOverview(%v) error = %v, want %q", tt.args, err, tt.want)
			}
			if got := deps.Result(); got != "" {
				t.Errorf("Result Output = %q, want empty", got)
			}
		})
	}
}

// TestExecRepoOverviewRendersReadme covers `gits repo-overview acme api`: the
// README's path heads the preview and its rendered text follows, both on
// Result Output.
func TestExecRepoOverviewRendersReadme(t *testing.T) {
	t.Parallel()

	deps := clitest.New(t, previewGit()).WithProject("acme", clitest.Cloned("api"))

	readmePath := filepath.Join(deps.Projects["acme"].Path, "api", ReadMeFilename)
	body := "# Fixture\n\nBrowsing a repository shows this.\n"
	if err := os.WriteFile(readmePath, []byte(body), 0o600); err != nil {
		t.Fatalf("write README: %v", err)
	}

	if err := ExecRepoOverview([]string{"acme", "api"}, deps.RuntimeCLI); err != nil {
		t.Fatalf("ExecRepoOverview error = %v, want nil", err)
	}

	got := deps.Result()
	for _, want := range []string{readmePath, "Fixture", "Browsing a repository shows this."} {
		if !strings.Contains(got, want) {
			t.Errorf("Result Output = %q, want it to contain %q", got, want)
		}
	}
	if got := deps.Diagnostic(); got != "" {
		t.Errorf("Diagnostic Output = %q, want empty", got)
	}
}

// TestExecRepoOverviewWithoutReadme covers a repository with no README: the
// preview says so rather than rendering an empty pane.
func TestExecRepoOverviewWithoutReadme(t *testing.T) {
	t.Parallel()

	deps := clitest.New(t, previewGit()).WithProject("acme", clitest.Cloned("api"))

	err := ExecRepoOverview([]string{"acme", "api"}, deps.RuntimeCLI)
	if err == nil || !strings.Contains(err.Error(), ReadMeFilename) {
		t.Fatalf("ExecRepoOverview error = %v, want it to name the missing %s", err, ReadMeFilename)
	}
	if got := deps.Result(); got != "" {
		t.Errorf("Result Output = %q, want empty — there was nothing to render", got)
	}
}

// TestExecRepoOverviewAbortsOnNonOKState covers the state guard: a Repository
// that is not `ok` has no work tree to read a README from.
func TestExecRepoOverviewAbortsOnNonOKState(t *testing.T) {
	t.Parallel()

	deps := clitest.New(t, previewGit()).WithProject("acme", clitest.Broken("bad"))

	err := ExecRepoOverview([]string{"acme", "bad"}, deps.RuntimeCLI)
	if err == nil {
		t.Fatal("ExecRepoOverview error = nil, want the state to abort the command")
	}
	if want := clitest.BrokenReason + "\n"; deps.Diagnostic() != want {
		t.Errorf("Diagnostic Output = %q, want exactly %q", deps.Diagnostic(), want)
	}
}

// TestBrowseTarget covers the argument dispatch of ExecBrowse: the project
// name must survive every arg count, and an explicit branch is only taken
// from a 3-arg invocation.
func TestBrowseTarget(t *testing.T) {
	t.Parallel()

	project := domain.Project{Name: "selected"}
	tests := []struct {
		name       string
		args       []string
		wantProj   string
		wantBranch string
	}{
		{"no args uses selected project", []string{}, "selected", ""},
		{"project only", []string{"acme"}, "acme", ""},
		{"project and repo", []string{"acme", "repo"}, "acme", ""},
		{"project repo branch", []string{"acme", "repo", "main"}, "acme", "main"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotProj, gotBranch := browseTarget(tt.args, project)
			if gotProj != tt.wantProj {
				t.Errorf("projName = %q, want %q", gotProj, tt.wantProj)
			}
			if gotBranch != tt.wantBranch {
				t.Errorf("branch = %q, want %q", gotBranch, tt.wantBranch)
			}
		})
	}
}
