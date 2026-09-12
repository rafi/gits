package cd

import (
	"path/filepath"
	"testing"

	"github.com/rafi/gits/internal/cli/clitest"
)

// Every test here drives ExecCD — the command's real entry point — with
// explicit arguments naming the project and the repository, so argument
// parsing runs for real and the interactive finder is never reached.

// TestExecCDWritesRepoPath is the shell integration's contract: `gits cd` emits
// one Repository path on Result Output and nothing else anywhere, so the shell
// function that consumes it can cd to what it read.
func TestExecCDWritesRepoPath(t *testing.T) {
	t.Parallel()

	deps := clitest.New(t, clitest.FakeGit{}).
		WithProject("acme", clitest.Cloned("api"), clitest.Cloned("web"))

	if err := ExecCD([]string{"acme", "api"}, deps.RuntimeCLI); err != nil {
		t.Fatalf("ExecCD error = %v, want nil", err)
	}

	want := filepath.Join(deps.Projects["acme"].Path, "api") + "\n"
	if got := deps.Result(); got != want {
		t.Errorf("Result Output = %q, want exactly %q", got, want)
	}
	if got := deps.Diagnostic(); got != "" {
		t.Errorf("Diagnostic Output = %q, want empty", got)
	}
}

// TestExecCDAbortsOnNonOKState covers the state guard: a Repository that is not
// `ok` aborts with its Reason on Diagnostic Output — terminated, so the next
// line starts on its own — and leaves Result Output empty, since a shell that
// captured a message here would try to cd into it.
func TestExecCDAbortsOnNonOKState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		repoName string
		repo     clitest.Repo
		want     string
	}{
		{"not cloned", "gone", clitest.NotCloned("gone"), "not cloned\n"},
		{"defective config", "bad", clitest.Broken("bad"), clitest.BrokenReason + "\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			deps := clitest.New(t, clitest.FakeGit{}).WithProject("acme", tt.repo)

			err := ExecCD([]string{"acme", tt.repoName}, deps.RuntimeCLI)
			if err == nil {
				t.Fatalf("ExecCD(%s) error = nil, want the state to abort the command", tt.repoName)
			}
			if got := deps.Diagnostic(); got != tt.want {
				t.Errorf("Diagnostic Output = %q, want exactly %q", got, tt.want)
			}
			if got := deps.Result(); got != "" {
				t.Errorf("Result Output = %q, want empty — a shell would cd into it", got)
			}
		})
	}
}
