package clone

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cli/clitest"
	"github.com/rafi/gits/internal/git"
)

// Every test here drives ExecClone — the command's real entry point — with
// explicit arguments, so argument parsing, the choice between walking a whole
// Project and acting on a single Repository, the error epilogue and the exit
// code all run for real, and the interactive finder is never reached.

// cloneCall records one clone: where it was cloned from and into.
type cloneCall struct {
	Src  string
	Repo string
}

// fakeGit implements the one call cloneRepo makes. The network success path is
// covered by manual demos (SPEC §5); what a test can check is which
// repositories were reached and what was made of the answer.
type fakeGit struct {
	clitest.FakeGit

	out string
	err error

	mu     sync.Mutex
	clones []cloneCall
}

// Clone mirrors the real client's one refusal: a target that already exists is
// never cloned into. That is what makes a fixture repository already in Repo
// State `ok` behave here as it would in a real run.
func (f *fakeGit) Clone(_ context.Context, src, path string) (string, error) {
	f.mu.Lock()
	f.clones = append(f.clones, cloneCall{src, filepath.Base(path)})
	f.mu.Unlock()
	if f.err != nil {
		return "", f.err
	}
	if _, err := os.Stat(path); err == nil {
		return "", git.ErrTargetExists
	}
	return f.out, nil
}

// Clones returns every clone, in the order the Traversal reached them.
func (f *fakeGit) Clones() []cloneCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.clones)
}

// TestExecCloneProject covers `gits clone acme` and the divergence that makes
// this command different from the other three: a repository in Repo State
// `not-cloned` is exactly what clone acts on, where the others pass over it. A
// repository already cloned is reached too and reports itself as such, without
// failing the run.
//
// Diagnostic Output being empty is the second assertion: a run with nothing but
// a warning to report emits no epilogue, and the live progress reporter —
// handed a buffer rather than a terminal — emits nothing at all, so no ANSI can
// reach any assertion in this package.
func TestExecCloneProject(t *testing.T) {
	t.Parallel()

	g := &fakeGit{out: "Cloning into 'gone'…"}
	deps := clitest.New(t, g).WithProject("acme", clitest.Cloned("api"), clitest.NotCloned("gone"))

	if err := ExecClone([]string{"acme"}, deps.RuntimeCLI); err != nil {
		t.Fatalf("ExecClone error = %v, want nil", err)
	}

	want := []cloneCall{
		{clitest.RepoSrc("api"), "api"},
		{clitest.RepoSrc("gone"), "gone"},
	}
	if !slices.Equal(g.Clones(), want) {
		t.Errorf("clones = %+v, want %+v", g.Clones(), want)
	}
	got := deps.Result()
	for _, want := range []string{"gone", "Cloning into", "already cloned"} {
		if !strings.Contains(got, want) {
			t.Errorf("Result Output = %q, want it to contain %q", got, want)
		}
	}
	if got := deps.Diagnostic(); got != "" {
		t.Errorf("Diagnostic Output = %q, want empty", got)
	}
}

// TestExecCloneSingleRepo covers `gits clone acme gone`: the second argument
// selects one repository, and only that one is cloned and rendered — without
// the project title the whole-project path prints.
func TestExecCloneSingleRepo(t *testing.T) {
	t.Parallel()

	g := &fakeGit{out: "Cloning into 'gone'…"}
	deps := clitest.New(t, g).WithProject("acme", clitest.Cloned("api"), clitest.NotCloned("gone"))

	if err := ExecClone([]string{"acme", "gone"}, deps.RuntimeCLI); err != nil {
		t.Fatalf("ExecClone error = %v, want nil", err)
	}

	want := []cloneCall{{clitest.RepoSrc("gone"), "gone"}}
	if !slices.Equal(g.Clones(), want) {
		t.Errorf("clones = %+v, want %+v", g.Clones(), want)
	}
	if got := deps.Result(); !strings.Contains(got, "Cloning into") {
		t.Errorf("Result Output = %q, want the selected repository's result", got)
	}
}

// TestExecCloneSkipsErrorState covers the one state clone does refuse: a
// repository whose configuration is defective has no destination to clone
// into, so it is passed over reporting the Reason it was classified with, and
// counts toward the exit code.
func TestExecCloneSkipsErrorState(t *testing.T) {
	t.Parallel()

	g := &fakeGit{out: "Cloning into 'gone'…"}
	deps := clitest.New(t, g).WithProject("acme", clitest.NotCloned("gone"), clitest.Broken("bad"))

	err := ExecClone([]string{"acme"}, deps.RuntimeCLI)
	if err == nil {
		t.Fatal("ExecClone error = nil, want the defective repository to fail the run")
	}

	if want := []cloneCall{{clitest.RepoSrc("gone"), "gone"}}; !slices.Equal(g.Clones(), want) {
		t.Errorf("clones = %+v, want only the clonable repository %+v", g.Clones(), want)
	}
	if got := deps.Result(); !strings.Contains(got, clitest.BrokenReason) {
		t.Errorf("Result Output = %q, want the repository's own Reason", got)
	}
	got := deps.Diagnostic()
	if !strings.Contains(got, "1 error:") || !strings.Contains(got, clitest.BrokenReason) {
		t.Errorf("Diagnostic Output = %q, want the error epilogue", got)
	}
}

// TestExecCloneFailureReportsEpilogue covers a repository whose clone fails:
// git's message reaches Result Output on the repository's line and Diagnostic
// Output in the error epilogue, and the run reports failure.
func TestExecCloneFailureReportsEpilogue(t *testing.T) {
	t.Parallel()

	g := &fakeGit{err: errors.New("repository not found")}
	deps := clitest.New(t, g).WithProject("acme", clitest.NotCloned("gone"))

	err := ExecClone([]string{"acme"}, deps.RuntimeCLI)
	if err == nil {
		t.Fatal("ExecClone error = nil, want the failed repository to fail the run")
	}
	if !strings.Contains(err.Error(), "completed with errors") {
		t.Errorf("ExecClone error = %v, want the run reported as failed", err)
	}
	if got := deps.Result(); !strings.Contains(got, "repository not found") {
		t.Errorf("Result Output = %q, want git's message on the repository line", got)
	}
	got := deps.Diagnostic()
	if !strings.Contains(got, "1 error:") || !strings.Contains(got, "repository not found") {
		t.Errorf("Diagnostic Output = %q, want the error epilogue", got)
	}
}

// hitCache reports every project as already cached, so the loader leaves the
// fixture's repositories exactly as declared instead of contacting a provider.
type hitCache struct{}

func (hitCache) Get(string, *domain.Project) (bool, error) { return true, nil }
func (hitCache) Save(string, domain.Project) error         { return nil }
func (hitCache) Flush(domain.Project) error                { return nil }

// TestExecCloneRemoteOnly covers the state clone must never hand to git: a
// provider-backed repository with no local home (remote-only). It is passed
// over with a warning naming the config keys that would give it a path, git is
// never reached, and the run's exit code is unaffected — a pass-over, not a
// failure.
func TestExecCloneRemoteOnly(t *testing.T) {
	t.Parallel()

	g := &fakeGit{out: "Cloning into 'api'…"}
	deps := clitest.New(t, g)
	deps.Cache = hitCache{}
	// No project path and a remote provider source, with the repository
	// declaring no dir: — the exact shape the loader classifies as remote-only.
	deps.Projects = domain.ProjectListKeyed{"acme": {
		Source: &domain.ProviderSource{Type: "github", Search: "acme"},
		Repos:  []domain.Repository{{Name: "api", Src: "git@github.com:acme/api.git"}},
	}}

	if err := ExecClone([]string{"acme"}, deps.RuntimeCLI); err != nil {
		t.Fatalf("ExecClone error = %v, want nil — remote-only is a pass-over", err)
	}

	if got := g.Clones(); len(got) != 0 {
		t.Errorf("clones = %+v, want none: a remote-only repo has no local path", got)
	}
	if got := deps.Result(); !strings.Contains(got, "no local path") ||
		!strings.Contains(got, "`path:`") || !strings.Contains(got, "`dir:`") {
		t.Errorf("Result Output = %q, want the line to name the config keys", got)
	}
	if got := deps.Diagnostic(); got != "" {
		t.Errorf("Diagnostic Output = %q, want empty — a pass-over is not an error", got)
	}
}

// skipping returns dependencies whose "acme" project is configured not to be
// cloned, which is the configuration both tests below turn on.
func skipping(t *testing.T, g *fakeGit, repos ...clitest.Repo) *clitest.Deps {
	t.Helper()
	deps := clitest.New(t, g).WithProject("acme", repos...)
	no := false
	project := deps.Projects["acme"]
	project.Clone = &no
	deps.Projects["acme"] = project
	return deps
}

// TestExecCloneSkippedProject covers `gits clone acme` on a project configured
// not to clone: nothing is cloned, and the skip is reported as Diagnostic
// Output rather than through a logger nothing else in a command reaches.
func TestExecCloneSkippedProject(t *testing.T) {
	t.Parallel()

	g := &fakeGit{out: "Cloning into 'gone'…"}
	deps := skipping(t, g, clitest.NotCloned("gone"))

	if err := ExecClone([]string{"acme"}, deps.RuntimeCLI); err != nil {
		t.Fatalf("ExecClone error = %v, want nil", err)
	}

	if got := g.Clones(); len(got) != 0 {
		t.Errorf("clones = %+v, want none under a project configured not to clone", got)
	}
	// The project vanishes rather than appearing as an empty block: its title
	// is not drawn either, so Result Output stays pipeable-empty.
	if got := deps.Result(); got != "" {
		t.Errorf("Result Output = %q, want a skipped project to render nothing", got)
	}
	if got := deps.Diagnostic(); !strings.Contains(got, "Skipping acme") {
		t.Errorf("Diagnostic Output = %q, want the skipped project named", got)
	}
}

// TestExecCloneSkippedProjectSingleRepo covers `gits clone acme gone` on the
// same project: naming a repository explicitly must not override the
// configuration that skips its project. Nothing is cloned, nothing reaches
// Result Output, and the skip is reported exactly as it is for the whole
// project.
func TestExecCloneSkippedProjectSingleRepo(t *testing.T) {
	t.Parallel()

	g := &fakeGit{out: "Cloning into 'gone'…"}
	deps := skipping(t, g, clitest.NotCloned("gone"))

	if err := ExecClone([]string{"acme", "gone"}, deps.RuntimeCLI); err != nil {
		t.Fatalf("ExecClone error = %v, want nil", err)
	}

	if got := g.Clones(); len(got) != 0 {
		t.Errorf("clones = %+v, want none: the configuration outranks the argument", got)
	}
	if got := deps.Result(); got != "" {
		t.Errorf("Result Output = %q, want nothing rendered for a skipped repository", got)
	}
	if got := deps.Diagnostic(); !strings.Contains(got, "Skipping acme") {
		t.Errorf("Diagnostic Output = %q, want the skipped project named", got)
	}
}

// TestExecCloneSkippedSubProject covers a sub-project configured not to clone:
// it takes everything beneath it with it, while the rest of the project clones
// as usual. A skipped project vanishes rather than failing — nothing under it
// counts toward the exit code.
func TestExecCloneSkippedSubProject(t *testing.T) {
	t.Parallel()

	g := &fakeGit{out: "Cloning into 'z'…"}
	deps := clitest.New(t, g)

	root := clitest.NewProject(t, clitest.NotCloned("z"))
	skipped := clitest.NewProject(t, clitest.NotCloned("y"))
	skipped.Name = "skip"
	no := false
	skipped.Clone = &no
	sub := clitest.NewProject(t, clitest.NotCloned("x"))
	sub.Name = "sub"
	skipped.SubProjects = []domain.Project{sub}
	root.SubProjects = []domain.Project{skipped}
	deps.Projects["acme"] = root

	if err := ExecClone([]string{"acme"}, deps.RuntimeCLI); err != nil {
		t.Fatalf("ExecClone error = %v, want nil", err)
	}

	want := []cloneCall{{clitest.RepoSrc("z"), "z"}}
	if !slices.Equal(g.Clones(), want) {
		t.Errorf("clones = %+v, want only the unskipped repository %+v", g.Clones(), want)
	}
	result := deps.Result()
	if !strings.Contains(result, "z") {
		t.Errorf("Result Output = %q, want the unskipped repository's line", result)
	}
	for _, banned := range []string{":: skip", ":: sub"} {
		if strings.Contains(result, banned) {
			t.Errorf("Result Output = %q, want %q to have vanished entirely", result, banned)
		}
	}
	if got := deps.Diagnostic(); !strings.Contains(got, "Skipping skip") {
		t.Errorf("Diagnostic Output = %q, want the skipped sub-project named", got)
	}
}
