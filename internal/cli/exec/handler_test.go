package exec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rafi/gits/internal/cli/clitest"
)

// Every test drives Exec — the command's real entry point — with explicit
// arguments, and runs a real child process. The children are the POSIX
// utilities every supported platform ships (`echo`, `pwd`, `false`, `sh`), so
// nothing here needs a fake: the point of this command is that a real process
// runs in a real directory.

// TestExecProject covers `gits exec acme -- echo hi`: the command runs in
// every repository and its output reaches Result Output under the repository's
// line.
func TestExecProject(t *testing.T) {
	t.Parallel()

	deps := clitest.New(t, clitest.FakeGit{}).
		WithProject("acme", clitest.Cloned("api"), clitest.Cloned("web"))

	err := Exec("table", []string{"echo", "hi"}, []string{"acme"}, deps.RuntimeCLI)
	if err != nil {
		t.Fatalf("ExecExec error = %v, want nil", err)
	}

	got := deps.Result()
	for _, want := range []string{"api", "web", "hi"} {
		if !strings.Contains(got, want) {
			t.Errorf("Result Output = %q, want it to contain %q", got, want)
		}
	}
	if got := deps.Diagnostic(); got != "" {
		t.Errorf("Diagnostic Output = %q, want empty", got)
	}
}

// TestExecRunsInRepoDir proves the child's working directory is the
// repository, not the process's own: `pwd` prints each repository's path.
func TestExecRunsInRepoDir(t *testing.T) {
	t.Parallel()

	deps := clitest.New(t, clitest.FakeGit{}).WithProject("acme", clitest.Cloned("api"))

	if err := Exec("table", []string{"pwd"}, []string{"acme"}, deps.RuntimeCLI); err != nil {
		t.Fatalf("ExecExec error = %v, want nil", err)
	}

	want := filepath.Join(deps.Projects["acme"].Path, "api")
	// macOS resolves the temporary directory through /private, so compare on
	// the resolved path rather than the one the fixture handed out.
	if resolved, err := filepath.EvalSymlinks(want); err == nil {
		want = resolved
	}
	if got := deps.Result(); !strings.Contains(got, want) {
		t.Errorf("Result Output = %q, want it to contain the repository path %q", got, want)
	}
}

// TestExecEnvironment covers the three variables a child is given: they
// name the repository it is running in.
func TestExecEnvironment(t *testing.T) {
	t.Parallel()

	deps := clitest.New(t, clitest.FakeGit{}).WithProject("acme", clitest.Cloned("api"))

	cmd := []string{"sh", "-c", "echo $GITS_PROJECT/$GITS_REPO@$GITS_REPO_PATH"}
	if err := Exec("table", cmd, []string{"acme"}, deps.RuntimeCLI); err != nil {
		t.Fatalf("ExecExec error = %v, want nil", err)
	}

	got := deps.Result()
	if !strings.Contains(got, "acme/api@") {
		t.Errorf("Result Output = %q, want GITS_PROJECT and GITS_REPO exported", got)
	}
	if !strings.Contains(got, filepath.Join(deps.Projects["acme"].Path, "api")) {
		t.Errorf("Result Output = %q, want GITS_REPO_PATH exported", got)
	}
}

// TestExecNonZeroExit covers a failing child: it is that repository's
// error, so it shows on its line, reaches the error epilogue and fails the
// run — while the other repository still ran.
func TestExecNonZeroExit(t *testing.T) {
	t.Parallel()

	deps := clitest.New(t, clitest.FakeGit{}).
		WithProject("acme", clitest.Cloned("api"), clitest.Cloned("web"))

	script := `test "$GITS_REPO" != api || { echo boom >&2; exit 3; }`
	err := Exec("table", []string{"sh", "-c", script}, []string{"acme"}, deps.RuntimeCLI)
	if err == nil {
		t.Fatal("ExecExec error = nil, want the failing command to fail the run")
	}

	if got := deps.Result(); !strings.Contains(got, "exit status 3") {
		t.Errorf("Result Output = %q, want the failing repository's exit status", got)
	}
	// The child's own output is kept: "exit status 3" alone names nothing.
	if got := deps.Result(); !strings.Contains(got, "boom") {
		t.Errorf("Result Output = %q, want the child's output alongside the status", got)
	}
	if got := deps.Diagnostic(); !strings.Contains(got, "api") {
		t.Errorf("Diagnostic Output = %q, want the failing repository named in the epilogue", got)
	}
}

// TestExecSingleRepo covers `gits exec acme api -- …`: the second argument
// selects one repository and only that one runs.
func TestExecSingleRepo(t *testing.T) {
	t.Parallel()

	deps := clitest.New(t, clitest.FakeGit{}).
		WithProject("acme", clitest.Cloned("api"), clitest.Cloned("web"))

	cmd := []string{"sh", "-c", "echo ran-$GITS_REPO"}
	if err := Exec("table", cmd, []string{"acme", "api"}, deps.RuntimeCLI); err != nil {
		t.Fatalf("ExecExec error = %v, want nil", err)
	}

	got := deps.Result()
	if !strings.Contains(got, "ran-api") {
		t.Errorf("Result Output = %q, want the selected repository to have run", got)
	}
	if strings.Contains(got, "ran-web") {
		t.Errorf("Result Output = %q, want nothing about the other repository", got)
	}
}

// TestExecSkipsNonOKRepositories covers the state guard: a repository with
// no clone on disk has no directory to run in, so it is passed over rather
// than run somewhere else.
func TestExecSkipsNonOKRepositories(t *testing.T) {
	t.Parallel()

	deps := clitest.New(t, clitest.FakeGit{}).
		WithProject("acme", clitest.Cloned("api"), clitest.NotCloned("gone"))

	marker := filepath.Join(t.TempDir(), "ran")
	cmd := []string{"sh", "-c", "echo $GITS_REPO >> " + marker}
	err := Exec("table", cmd, []string{"acme"}, deps.RuntimeCLI)
	if err == nil {
		t.Fatal("ExecExec error = nil, want the skipped repository to fail the run")
	}

	ran, readErr := os.ReadFile(marker)
	if readErr != nil {
		t.Fatalf("read marker: %v", readErr)
	}
	if got := strings.Fields(string(ran)); len(got) != 1 || got[0] != "api" {
		t.Errorf("commands ran in %v, want only [api]", got)
	}
	if got := deps.Result(); !strings.Contains(got, "not cloned") {
		t.Errorf("Result Output = %q, want the skipped repository's reason", got)
	}
}

// TestExecNoCommand covers the empty argv: nothing runs and the error says
// how to invoke the command, rather than the run reporting success over an
// empty command.
func TestExecNoCommand(t *testing.T) {
	t.Parallel()

	deps := clitest.New(t, clitest.FakeGit{}).WithProject("acme", clitest.Cloned("api"))

	err := Exec("table", nil, []string{"acme"}, deps.RuntimeCLI)
	if err == nil {
		t.Fatal("ExecExec error = nil, want ErrNoCommand")
	}
	if got := deps.Result(); got != "" {
		t.Errorf("Result Output = %q, want empty", got)
	}
}

// TestExecNoShellInterpretation proves the argv path: a metacharacter in
// an argument is passed to the child literally instead of being interpreted.
func TestExecNoShellInterpretation(t *testing.T) {
	t.Parallel()

	deps := clitest.New(t, clitest.FakeGit{}).WithProject("acme", clitest.Cloned("api"))

	err := Exec("table", []string{"echo", "a; touch pwned", "$HOME"}, []string{"acme"}, deps.RuntimeCLI)
	if err != nil {
		t.Fatalf("ExecExec error = %v, want nil", err)
	}

	if got := deps.Result(); !strings.Contains(got, "a; touch pwned $HOME") {
		t.Errorf("Result Output = %q, want the arguments passed through literally", got)
	}
	pwned := filepath.Join(deps.Projects["acme"].Path, "api", "pwned")
	if _, err := os.Stat(pwned); err == nil {
		t.Error("the shell metacharacter was interpreted; argv must be run directly")
	}
}

// TestExecJSON covers `gits exec -o json acme -- …`: the child's output nests
// under `exec` as output, a non-zero exit as error carrying the child's own
// output alongside the status, and a repository with no clone — which the
// command never runs in — carries its state and no such object. None of it
// fails the run or prints an epilogue in this format.
func TestExecJSON(t *testing.T) {
	t.Parallel()

	deps := clitest.New(t, clitest.FakeGit{}).
		WithProject("acme", clitest.Cloned("api"), clitest.Cloned("web"), clitest.NotCloned("gone"))

	script := `test "$GITS_REPO" != api || { echo boom; exit 3; }; echo ok $GITS_REPO`
	err := Exec("json", []string{"sh", "-c", script}, []string{"acme"}, deps.RuntimeCLI)
	if err != nil {
		t.Fatalf("Exec error = %v, want nil: a repository's condition is data", err)
	}
	if got := deps.Diagnostic(); got != "" {
		t.Errorf("Diagnostic Output = %q, want no epilogue", got)
	}

	repos := deps.JSONRepos("acme")
	web, ok := repos["web"]["exec"].(map[string]any)
	if !ok || len(web) != 1 || web["output"] != "ok web" {
		t.Errorf("web.exec = %v, want only the child's output", repos["web"]["exec"])
	}
	api, ok := repos["api"]["exec"].(map[string]any)
	if msg, _ := api["error"].(string); !ok || len(api) != 1 ||
		!strings.Contains(msg, "exit status 3") || !strings.Contains(msg, "boom") {
		t.Errorf("api.exec = %v, want only the error, carrying the status and the child's output",
			repos["api"]["exec"])
	}
	if _, found := repos["gone"]["exec"]; found {
		t.Errorf("gone = %v, want no outcome for a repository with no directory to run in", repos["gone"])
	}
}

// TestExecUnknownFormat covers a format other than table or json being
// rejected before anything is loaded — and before the argv is checked, so an
// empty command is not what gets reported.
func TestExecUnknownFormat(t *testing.T) {
	t.Parallel()

	deps := clitest.New(t, clitest.FakeGit{}).WithProject("acme", clitest.Cloned("api"))

	err := Exec("name", nil, []string{"acme"}, deps.RuntimeCLI)
	if err == nil || !strings.Contains(err.Error(), "unknown output format") {
		t.Fatalf("Exec(\"name\") error = %v, want the format rejected", err)
	}
	if got := deps.Result() + deps.Diagnostic(); got != "" {
		t.Errorf("output = %q, want nothing rendered", got)
	}
}
