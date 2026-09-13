package pick

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/app"
	"github.com/rafi/gits/internal/app/cli/style"
	"github.com/rafi/gits/internal/cli/clitest"
	"github.com/rafi/gits/internal/git"
)

// fakeGit stubs the git.Reader methods project population touches. Finders
// only read, so the fake is a Reader and cannot satisfy a write at all.
type fakeGit struct {
	git.Reader
	clitest.FakeNoWrites
}

func (fakeGit) Remote(context.Context, string) (string, error) { return "git@x:a/b.git", nil }
func (fakeGit) IsRepo(context.Context, string) bool            { return true }

// stubCache is a no-op Cacher.
type stubCache struct{}

func (stubCache) Get(string, *domain.Project) (bool, error) { return false, nil }
func (stubCache) Save(string, domain.Project) error         { return nil }
func (stubCache) Flush(domain.Project) error                { return nil }

func finderDeps(t *testing.T) app.RuntimeCLI {
	t.Helper()
	return app.RuntimeCLI{
		Ctx:   context.Background(),
		Git:   fakeGit{},
		Cache: stubCache{},
		Projects: domain.ProjectListKeyed{
			"myproj": {
				Repos: []domain.Repository{
					{Dir: "~/code/alpha", Src: "git@x:acme/alpha.git"},
					{Dir: "~/code/bravo", Src: "git@x:acme/bravo.git"},
				},
			},
		}}
}

// TestParseArgsNonInteractive covers every ParseArgs path that does not need
// an interactive finder.
func TestParseArgsNonInteractive(t *testing.T) {
	t.Parallel()

	t.Run("project and repo by name", func(t *testing.T) {
		t.Parallel()

		deps := finderDeps(t)
		proj, repo, err := ParseArgs([]string{"myproj", "alpha"}, true, deps)
		if err != nil {
			t.Fatalf("ParseArgs: %v", err)
		}
		if proj.Name != "myproj" {
			t.Errorf("project = %q, want myproj", proj.Name)
		}
		if repo == nil || repo.GetName() != "alpha" {
			t.Errorf("repo = %+v, want alpha", repo)
		}
	})

	t.Run("repo arg wins even without forced selection", func(t *testing.T) {
		t.Parallel()

		deps := finderDeps(t)
		_, repo, err := ParseArgs([]string{"myproj", "bravo"}, true, deps)
		if err != nil {
			t.Fatalf("ParseArgs: %v", err)
		}
		if repo == nil || repo.GetName() != "bravo" {
			t.Errorf("repo = %+v, want bravo", repo)
		}
	})

	t.Run("unknown repo errors", func(t *testing.T) {
		t.Parallel()

		deps := finderDeps(t)
		_, _, err := ParseArgs([]string{"myproj", "nope"}, true, deps)
		if err == nil {
			t.Fatal("ParseArgs(unknown repo) = nil, want error")
		}
	})

	t.Run("skip select without repo arg returns nil repo", func(t *testing.T) {
		t.Parallel()

		deps := finderDeps(t)
		proj, repo, err := ParseArgs([]string{"myproj"}, true, deps)
		if err != nil {
			t.Fatalf("ParseArgs: %v", err)
		}
		if proj.Name != "myproj" || repo != nil {
			t.Errorf("ParseArgs = (%q, %+v), want (myproj, nil repo)", proj.Name, repo)
		}
	})

	t.Run("unknown project errors, not warns", func(t *testing.T) {
		t.Parallel()

		deps := finderDeps(t)
		_, _, err := ParseArgs([]string{"ghost"}, true, deps)
		if err == nil {
			t.Fatal("ParseArgs(ghost) = nil, want error")
		}
		if domain.IsWarning(err) {
			t.Errorf("ParseArgs(ghost) is a downgradeable warning, want a real error: %v", err)
		}
	})
}

// stubFinder installs a fake fzf at the front of PATH. The script receives the
// candidate lines on stdin, so a shim that echoes one drives the real
// FZF.Project/FZF.Repo/SelectBranch end to end.
func stubFinder(t *testing.T, script string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "fzf")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script+"\n"), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// selectDeps is finderDeps with the output destinations and theme a finder
// needs, plus a named project so titles render.
func selectDeps(t *testing.T) app.RuntimeCLI {
	t.Helper()
	deps := finderDeps(t)
	deps.Theme = style.NewThemeDefault()
	deps.Out = io.Discard
	deps.Err = io.Discard
	return deps
}

//nolint:paralleltest // stubFinder calls t.Setenv, which t.Parallel forbids.
func TestFZFProject(t *testing.T) {
	t.Run("returns the first field of the chosen line", func(t *testing.T) {
		// The project title carries styling and may carry a source and
		// description; FZF.Project keeps only the name.
		stubFinder(t, "head -n1")

		got, err := NewFZF(selectDeps(t)).Project(t.Context())
		if err != nil {
			t.Fatalf("FZF.Project: %v", err)
		}
		// The picker returns the styled line, and FZF.Project keeps its
		// first field; the name is what the user sees inside the styling.
		if stripANSI(got) != "myproj" {
			t.Errorf("FZF.Project() = %q, want %q", stripANSI(got), "myproj")
		}
	})

	t.Run("an abort selects nothing without failing", func(t *testing.T) {
		stubFinder(t, "exit 130")

		got, err := NewFZF(selectDeps(t)).Project(t.Context())
		if err != nil {
			t.Fatalf("FZF.Project: %v, want a cancellation to be silent", err)
		}
		if got != "" {
			t.Errorf("FZF.Project() = %q, want empty on abort", got)
		}
	})

	t.Run("a finder failure is reported", func(t *testing.T) {
		stubFinder(t, "exit 2")

		if _, err := NewFZF(selectDeps(t)).Project(t.Context()); err == nil {
			t.Fatal("FZF.Project error = nil, want the finder's failure")
		}
	})
}

//nolint:paralleltest // stubFinder calls t.Setenv, which t.Parallel forbids.
func TestFZFRepo(t *testing.T) {
	t.Run("returns the chosen repository name", func(t *testing.T) {
		stubFinder(t, "head -n1")

		deps := selectDeps(t)
		project := deps.Projects["myproj"]
		project.Name = "myproj"
		got, err := NewFZF(deps).Repo(t.Context(), project, "")
		if err != nil {
			t.Fatalf("FZF.Repo: %v", err)
		}
		if stripANSI(got) != "alpha" {
			t.Errorf("FZF.Repo() = %q, want %q", stripANSI(got), "alpha")
		}
	})

	t.Run("an abort selects nothing without failing", func(t *testing.T) {
		stubFinder(t, "exit 130")

		deps := selectDeps(t)
		got, err := NewFZF(deps).Repo(t.Context(), deps.Projects["myproj"], "")
		if err != nil {
			t.Fatalf("FZF.Repo: %v, want a cancellation to be silent", err)
		}
		if got != "" {
			t.Errorf("FZF.Repo() = %q, want empty on abort", got)
		}
	})

	t.Run("a finder failure is reported", func(t *testing.T) {
		stubFinder(t, "exit 2")

		deps := selectDeps(t)
		if _, err := NewFZF(deps).Repo(t.Context(), deps.Projects["myproj"], ""); err == nil {
			t.Fatal("FZF.Repo error = nil, want the finder's failure")
		}
	})
}

//nolint:paralleltest // stubFinder calls t.Setenv, which t.Parallel forbids.
func TestSelectBranch(t *testing.T) {
	repo := domain.Repository{Name: "alpha", Namespace: "acme", AbsPath: "/tmp/alpha"}

	t.Run("a branch is returned without its label", func(t *testing.T) {
		// SelectBranch rewrites refs/heads/ and refs/tags/ into labeled
		// columns, then returns the ref name from the second column.
		stubFinder(t, "head -n1")

		deps := selectDeps(t)
		deps.Git = refsGit{refs: []string{"refs/heads/main", "refs/tags/v1.0.0"}}
		got, err := SelectBranch("myproj", repo, deps)
		if err != nil {
			t.Fatalf("SelectBranch: %v", err)
		}
		if got != "main" {
			t.Errorf("SelectBranch() = %q, want %q", got, "main")
		}
	})

	t.Run("a tag is returned without its label", func(t *testing.T) {
		stubFinder(t, "tail -n1")

		deps := selectDeps(t)
		deps.Git = refsGit{refs: []string{"refs/heads/main", "refs/tags/v1.0.0"}}
		got, err := SelectBranch("myproj", repo, deps)
		if err != nil {
			t.Fatalf("SelectBranch: %v", err)
		}
		if got != "v1.0.0" {
			t.Errorf("SelectBranch() = %q, want %q", got, "v1.0.0")
		}
	})

	// A canceled branch prompt is a documented pass-over, not a failure, so
	// it must come back as a domain.Warning rather than a plain error.
	t.Run("an abort is a warning", func(t *testing.T) {
		stubFinder(t, "exit 130")

		deps := selectDeps(t)
		deps.Git = refsGit{refs: []string{"refs/heads/main"}}
		_, err := SelectBranch("myproj", repo, deps)
		if err == nil {
			t.Fatal("SelectBranch error = nil, want a warning")
		}
		if !domain.IsWarning(err) {
			t.Errorf("SelectBranch error = %T (%v), want a downgradeable warning", err, err)
		}
	})

	// The message must name what for-each-ref was asked for, not claim the
	// repository could not be opened.
	t.Run("a Refs failure names branches and tags", func(t *testing.T) {
		stubFinder(t, "head -n1")

		deps := selectDeps(t)
		deps.Git = refsGit{err: errors.New("boom")}
		_, err := SelectBranch("myproj", repo, deps)
		if err == nil {
			t.Fatal("SelectBranch error = nil, want the Refs failure")
		}
		if !strings.Contains(err.Error(), "unable to list branches and tags") {
			t.Errorf("SelectBranch error = %q, want it to name branches and tags", err)
		}
	})
}

// refsGit answers Refs, which only SelectBranch needs.
type refsGit struct {
	fakeGit

	refs []string
	err  error
}

func (g refsGit) Refs(context.Context, string) ([]string, error) {
	return g.refs, g.err
}

// stripANSI removes SGR escape sequences, so an assertion can be made on the
// text a user sees rather than on the styling lipgloss wraps it in.
func stripANSI(s string) string {
	return ansiPattern.ReplaceAllString(s, "")
}

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)
