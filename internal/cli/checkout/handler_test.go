package checkout

import (
	"context"
	"errors"
	"io"
	"testing"

	log "github.com/sirupsen/logrus"

	"github.com/rafi/gits/internal/types"
	"github.com/rafi/gits/pkg/git"
)

var (
	errBoom     = errors.New("boom")
	errFakeExit = errors.New("fake exit")
)

// fakeBranchClient satisfies git.GitClient via the embedded nil interface;
// only the two methods promptRepo reaches before the (TTY) prompt are
// implemented. CurrentBranch succeeds so control flows into Branches, which
// fails — exercising the error path under test.
type fakeBranchClient struct {
	git.GitClient
}

func (fakeBranchClient) CurrentBranch(context.Context, string) (string, error) {
	return "main", nil
}

func (fakeBranchClient) Branches(context.Context, string) ([]string, error) {
	return nil, errBoom
}

// TestPromptRepoBranchesErrorDoesNotExit proves T3 (#4): a Branches failure
// surfaces as a returned error instead of aborting the whole process via
// log.Fatal / os.Exit. logrus's exit is neutralized so the buggy path (if
// present) records the exit and panics before the TTY prompt, rather than
// killing the test binary.
func TestPromptRepoBranchesErrorDoesNotExit(t *testing.T) {
	std := log.StandardLogger()
	origExit, origOut := std.ExitFunc, std.Out
	exited := false
	std.ExitFunc = func(int) { exited = true; panic(errFakeExit) }
	std.SetOutput(io.Discard) // swallow the Fatal log line
	t.Cleanup(func() { std.ExitFunc = origExit; std.SetOutput(origOut) })

	deps := types.RuntimeCLI{
		Runtime: types.Runtime{Ctx: context.Background(), Git: fakeBranchClient{}},
	}

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
		got, err = promptRepo("title", "/path", deps)
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
