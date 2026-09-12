// Package clitest builds what a command test needs to drive a command's real
// entry point: the shared runtime dependencies — both output destinations
// captured in buffers, the default theme, and the caller's fake git client —
// and a fixture project for the command to work on. Command packages construct
// types.RuntimeCLI through it rather than each carrying its own copy of this
// wiring, so a new dependency is added in one place.
//
// Buffers are not terminals, so the bulk module selects the no-op progress
// reporter and no ANSI cursor control reaches the captured text. Result and
// Diagnostic are additionally ANSI-stripped, since the theme still colors
// what it renders.
package clitest

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cli/config"
	"github.com/rafi/gits/internal/git"
	"github.com/rafi/gits/internal/logging"
	"github.com/rafi/gits/internal/types"
)

// HomeDir is the home directory tests render paths against. It never exists on
// disk: a fixture project's real paths live under a t.TempDir of their own (see
// NewProject), so nothing here renders relative to this one.
const HomeDir = "/home/nobody"

// Deps is a types.RuntimeCLI wired for a test, holding the buffers its two
// output destinations write to. The embedded struct is what a command entry
// point takes, and its fields stay settable — a test that needs a different
// project list, cache or worker count assigns one after New.
type Deps struct {
	types.RuntimeCLI

	t   *testing.T
	out bytes.Buffer
	err bytes.Buffer
	// log captures debug tracing separately from Diagnostic Output, so a
	// trace line can never be mistaken for something the user was shown.
	log bytes.Buffer
}

// New builds test dependencies around gitClient, which may be nil when the
// command under test never reaches the git client — any call on a nil client
// panics, which is the intended guard.
func New(t *testing.T, gitClient git.Client) *Deps {
	t.Helper()

	settings := domain.Settings{WorkerCount: 1}
	settings.Icons.ApplyDefaults()

	deps := &Deps{t: t}
	logger := logging.New(&deps.log, true)
	deps.RuntimeCLI = types.RuntimeCLI{
		Theme:   config.NewThemeDefault(),
		HomeDir: HomeDir,
		Out:     &deps.out,
		Err:     &deps.err,
		Runtime: types.Runtime{
			// Canceled when the test ends, so a command left walking does
			// not outlive it.
			Ctx:      t.Context(),
			Git:      gitClient,
			Settings: settings,
			Projects: domain.ProjectListKeyed{},
			Cache:    stubCache{},
			Log:      logger,
		},
	}
	return deps
}

// WithProject registers a fixture project under name — see NewProject — and
// returns the same Deps, so a test that drives a command over one project sets
// itself up in a single call.
func (d *Deps) WithProject(name string, repos ...Repo) *Deps {
	d.t.Helper()
	d.Projects[name] = NewProject(d.t, repos...)
	return d
}

// Result returns the captured Result Output with ANSI stripped.
func (d *Deps) Result() string { return ansi.Strip(d.out.String()) }

// Diagnostic returns the captured Diagnostic Output with ANSI stripped.
func (d *Deps) Diagnostic() string { return ansi.Strip(d.err.String()) }

// Trace returns the captured debug log records, which are not Diagnostic
// Output and never appear in it.
func (d *Deps) Trace() string { return d.log.String() }

// FakeGit is the base of a command test's fake git client: it answers the one
// question repository classification asks of git — whether a path is a git
// repository — and embeds the read-only half of the seam, so any query the
// test did not intend panics on the nil embed. A command's own fake embeds it
// and implements the calls that command makes.
//
// The write half is not embedded but implemented by FakeNoWrites, which panics
// by name: a read-only command's test fake cannot quietly satisfy a write, and
// a command that unexpectedly writes says which operation it reached instead of
// failing as a nil interface. A test for a writing command overrides the one
// method that command uses.
type FakeGit struct {
	git.Reader
	FakeNoWrites
}

// IsRepo reports every path as a git repository. Classification stats the path
// first, and NewProject creates a directory only for a repository it declares
// cloned, so whether a fixture repository is `ok` is already decided by then.
func (FakeGit) IsRepo(context.Context, string) bool { return true }

// FakeNoWrites satisfies git.Writer by refusing: every method panics naming
// itself. Embed it in a fake built around a bare git.Reader to get a full
// git.Client that still cannot write unnoticed.
type FakeNoWrites struct{}

// Clone panics: a fake that has not overridden it was never meant to write.
func (FakeNoWrites) Clone(context.Context, string, string) (string, error) {
	panic("unexpected git write: Clone")
}

// Fetch panics: a fake that has not overridden it was never meant to write.
func (FakeNoWrites) Fetch(context.Context, string) (string, error) {
	panic("unexpected git write: Fetch")
}

// Pull panics: a fake that has not overridden it was never meant to write.
func (FakeNoWrites) Pull(context.Context, string) (string, error) {
	panic("unexpected git write: Pull")
}

// Push panics: a fake that has not overridden it was never meant to write.
func (FakeNoWrites) Push(context.Context, string, git.PushTarget, git.PushOptions) (string, error) {
	panic("unexpected git write: Push")
}

// Checkout panics: a fake that has not overridden it was never meant to write.
func (FakeNoWrites) Checkout(context.Context, string, string) error {
	panic("unexpected git write: Checkout")
}

// brokenDir is a Repo Dir naming another user's home directory, which cannot be
// expanded and so leaves the repository with no local path at all.
const brokenDir = "~nobody-else/"

// BrokenReason is the Reason a Broken fixture repository carries. A test that
// asserts on it is proving the repository's own Reason reached the user rather
// than a generic "not a repository".
const BrokenReason = "unable to expand path: cannot expand user-specific home dir"

// RepoSrc returns the Repo Src NewProject gives the repository named name. It
// resolves nowhere: no fixture ever reaches a network, and a test asserting
// that a clone was made from it is asserting on what the config said.
func RepoSrc(name string) string { return "git@example.com:fixture/" + name + ".git" }

// Repo declares one repository of a fixture project. Build one with Cloned,
// NotCloned or Broken — the three Repo States NewProject can put a repository
// in.
type Repo struct {
	name  string
	state domain.RepoState
}

// Cloned declares a repository that classifies as `ok`: NewProject creates its
// directory, which is the only way to reach that state, since classification
// stats the path before asking the git client anything.
func Cloned(name string) Repo { return Repo{name: name, state: domain.RepoStateOK} }

// NotCloned declares a repository that classifies as `not-cloned`: its local
// directory is named and nothing is created there. This is the state `clone`
// acts on and every other bulk command passes over.
func NotCloned(name string) Repo { return Repo{name: name, state: domain.RepoStateNotCloned} }

// Broken declares a repository that classifies as `error`, carrying
// BrokenReason: its configured Repo Dir names no directory it could live in.
func Broken(name string) Repo { return Repo{name: name, state: domain.RepoStateError} }

// NewProject returns a fixture project carrying one repository per entry in
// repos, its Project Path a fresh temporary directory. The project is
// config-shaped — what a test assigns to Deps.Projects — and declares no
// Provider Source, so the loader classifies it without contacting one.
//
// The temporary directory is why these fixtures touch the filesystem at all:
// no repository reaches Repo State `ok` without a real path to stat.
func NewProject(t *testing.T, repos ...Repo) domain.Project {
	t.Helper()

	root := t.TempDir()
	project := domain.Project{Path: root}
	for _, r := range repos {
		repo := domain.Repository{Name: r.name, Dir: r.name, Src: RepoSrc(r.name)}
		switch r.state {
		case domain.RepoStateOK:
			if err := os.Mkdir(filepath.Join(root, r.name), 0o750); err != nil {
				t.Fatalf("create fixture repository %q: %v", r.name, err)
			}
		case domain.RepoStateNotCloned:
			// Nothing on disk: the named-but-absent directory is the state.
		case domain.RepoStateError:
			repo.Dir = brokenDir + r.name
		case domain.RepoStateUnknown, domain.RepoStateRemoteOnly:
			// Neither state is a fixture a test can ask for: unknown is the
			// pre-classification zero value, and remote-only is what the
			// loader decides from a Provider Source.
			fallthrough
		default:
			t.Fatalf("fixture repository %q: unsupported state %q", r.name, r.state)
		}
		project.Repos = append(project.Repos, repo)
	}
	return project
}

// stubCache is a no-op Cacher: nothing is ever cached, so every lookup misses
// and every save succeeds.
type stubCache struct{}

func (stubCache) Get(string, *domain.Project) (bool, error) { return false, nil }
func (stubCache) Save(string, domain.Project) error         { return nil }
func (stubCache) Flush(domain.Project) error                { return nil }
