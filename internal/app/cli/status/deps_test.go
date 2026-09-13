package status

import (
	"context"
	"testing"

	"github.com/rafi/gits/internal/app"
	"github.com/rafi/gits/internal/cli/clitest"
	"github.com/rafi/gits/internal/git"
)

// fakeGit stubs the status-relevant git.Reader methods. The view's own tests
// only need the probe to answer; every write panics by name.
type fakeGit struct {
	git.Reader
	clitest.FakeNoWrites

	snap        git.Snapshot
	workingDiff git.DiffStat
}

func (f fakeGit) Snapshot(context.Context, string) (git.Snapshot, error) {
	return f.snap, nil
}

func (f fakeGit) WorkingDiff(context.Context, string) (git.DiffStat, error) {
	return f.workingDiff, nil
}

func (fakeGit) FallbackRef(context.Context, string, string) string { return "" }

func (fakeGit) HeadInfo(context.Context, string) (git.Head, error) {
	return git.Head{}, nil
}

// statusDeps builds the shared runtime dependencies for the tests that render
// directly — nothing here writes to a destination, so only the theme, the
// icons and the git client matter.
func statusDeps(t *testing.T, g git.Client) app.RuntimeCLI {
	t.Helper()
	return clitest.New(t, g).RuntimeCLI
}
