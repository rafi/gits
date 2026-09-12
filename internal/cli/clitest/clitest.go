// Package clitest builds what a command test needs to drive a command's real
// entry point: the shared runtime dependencies — both output destinations
// captured in buffers, the default theme, and the caller's fake git client —
// and a fixture project for the command to work on. Command packages construct
// types.RuntimeCLI through it rather than each carrying its own copy of this
// wiring, so a new dependency is added in one place.
//
// Buffers are not terminals, so walk.NewReporter selects the no-op progress
// reporter and no ANSI cursor control reaches the captured text. Result and
// Diagnostic are additionally ANSI-stripped, since the theme still colours
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
	"github.com/rafi/gits/internal/types"
	"github.com/rafi/gits/pkg/git"
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
}

// New builds test dependencies around gitClient, which may be nil when the
// command under test never reaches the git client — any call on a nil client
// panics, which is the intended guard.
func New(t *testing.T, gitClient git.GitClient) *Deps {
	t.Helper()

	settings := domain.Settings{WorkerCount: 1}
	settings.Icons.ApplyDefaults()

	deps := &Deps{t: t}
	deps.RuntimeCLI = types.RuntimeCLI{
		Theme:   config.NewThemeDefault(),
		HomeDir: HomeDir,
		Out:     &deps.out,
		Err:     &deps.err,
		Runtime: types.Runtime{
			// Cancelled when the test ends, so a command left walking does
			// not outlive it.
			Ctx:      t.Context(),
			Git:      gitClient,
			Settings: settings,
			Projects: domain.ProjectListKeyed{},
			Cache:    stubCache{},
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

// FakeGit is the base of a command test's fake git client: it answers the one
// question repository classification asks of git — whether a path is a git
// repository — and embeds the interface, so any call the test did not intend
// panics on the nil embed. A command's own fake embeds it and implements the
// calls that command makes.
type FakeGit struct{ git.GitClient }

// IsRepo reports every path as a git repository. Classification stats the path
// first, and NewProject creates a directory only for a repository it declares
// cloned, so whether a fixture repository is `ok` is already decided by then.
func (FakeGit) IsRepo(context.Context, string) bool { return true }

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
