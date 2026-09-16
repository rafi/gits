package discover

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/app/cli/clitest"
	"github.com/rafi/gits/internal/config"
)

// Tests drive ExecDiscover over fixture repositories and a temp config file.

// fakeGit treats only directories containing .git as repositories.
type fakeGit struct {
	clitest.FakeGit
}

func (fakeGit) IsRepo(_ context.Context, path string) bool {
	_, err := os.Stat(filepath.Join(path, ".git"))
	return err == nil
}

// Remote returns a URL built from the directory name, or an error for
// `no-remote`.
func (fakeGit) Remote(_ context.Context, path string) (string, error) {
	if filepath.Base(path) == "no-remote" {
		return "", errors.New("no remote configured")
	}
	return "git@example.com:o/" + filepath.Base(path) + ".git", nil
}

// tree creates git repositories under a new temp directory and returns it.
func tree(t *testing.T, paths ...string) string {
	t.Helper()
	root := t.TempDir()
	repos(t, root, paths...)
	return root
}

// repos creates a git repository at each path relative to root.
func repos(t *testing.T, root string, paths ...string) {
	t.Helper()
	for _, path := range paths {
		if err := os.MkdirAll(filepath.Join(root, path, ".git"), 0o750); err != nil {
			t.Fatalf("create fixture repository %q: %v", path, err)
		}
	}
}

// configFile writes a config file holding content and returns its path.
func configFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

// readConfig returns a config file's content.
func readConfig(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config back: %v", err)
	}
	return string(data)
}

// TestExecDiscoverDryRunWritesNothing checks that -n reports without writing.
func TestExecDiscoverDryRunWritesNothing(t *testing.T) {
	t.Parallel()

	root := tree(t, "work/api", "work/web")
	const before = "# nothing here yet\n"
	deps := clitest.New(t, fakeGit{})
	deps.ConfigPath = configFile(t, before)

	if err := ExecDiscover(Options{DryRun: true}, []string{root}, deps.RuntimeCLI); err != nil {
		t.Fatalf("ExecDiscover error = %v, want nil", err)
	}

	if got := readConfig(t, deps.ConfigPath); got != before {
		t.Errorf("config file = %q, want it untouched at %q", got, before)
	}
	if got := deps.Result(); !strings.Contains(got, "work") {
		t.Errorf("Result Output = %q, want the group it would add", got)
	}
	if want := "Nothing was written"; !strings.Contains(deps.Diagnostic(), want) {
		t.Errorf("Diagnostic Output = %q, want it to say %q", deps.Diagnostic(), want)
	}
}

// TestExecDiscoverKeepsTheFileIntact checks that existing formatting and
// comments survive a write.
func TestExecDiscoverKeepsTheFileIntact(t *testing.T) {
	t.Parallel()

	root := tree(t, "oss/a", "oss/b")
	const before = `# my config

dotfiles:
  repos:
    - dir: ~/.config   # trailing comment
      src: "git@x:a/config.git"
`
	deps := clitest.New(t, fakeGit{})
	deps.ConfigPath = configFile(t, before)

	if err := ExecDiscover(Options{}, []string{root}, deps.RuntimeCLI); err != nil {
		t.Fatalf("ExecDiscover error = %v, want nil", err)
	}

	got := readConfig(t, deps.ConfigPath)
	if !strings.HasPrefix(got, before) {
		t.Errorf("config file = %q, want the original kept byte for byte with the project appended", got)
	}
	if !strings.Contains(got, "oss:") {
		t.Errorf("config file is missing the discovered project:\n%s", got)
	}
}

// TestExecDiscoverSkipsATakenName checks that a taken name is reported, not
// written.
func TestExecDiscoverSkipsATakenName(t *testing.T) {
	t.Parallel()

	root := tree(t, "work/a", "work/b", "oss/c", "oss/d")
	deps := clitest.New(t, fakeGit{})
	deps.ConfigPath = configFile(t, "work:\n  path: ~/elsewhere\n")
	deps.Projects = domain.ProjectListKeyed{
		"work": {Name: "work", Path: "~/elsewhere"},
	}

	if err := ExecDiscover(Options{}, []string{root}, deps.RuntimeCLI); err != nil {
		t.Fatalf("ExecDiscover error = %v, want nil", err)
	}

	config := readConfig(t, deps.ConfigPath)
	if strings.Contains(config, filepath.Join(root, "work")) {
		t.Errorf("config holds the group whose name was taken:\n%s", config)
	}
	if !strings.Contains(config, "oss:") {
		t.Errorf("config is missing the group whose name was free:\n%s", config)
	}
	if want := "already exists"; !strings.Contains(deps.Diagnostic(), want) {
		t.Errorf("Diagnostic Output = %q, want it to report the taken name", deps.Diagnostic())
	}
}

// TestExecDiscoverNothingToAddIsAWarning checks that an empty run warns with
// its reason instead of failing.
func TestExecDiscoverNothingToAddIsAWarning(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		paths []string
		min   int
		want  string
	}{
		{
			name:  "no repositories at all",
			paths: nil,
			want:  "no git repositories found",
		},
		{
			name:  "every directory holds fewer than --min",
			paths: []string{"a/one", "b/one"},
			min:   2,
			want:  "or more repositories",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			root := tree(t, tt.paths...)
			deps := clitest.New(t, fakeGit{})
			deps.ConfigPath = configFile(t, "")

			err := ExecDiscover(Options{Min: tt.min}, []string{root}, deps.RuntimeCLI)
			if err == nil {
				t.Fatal("ExecDiscover error = nil, want a warning that nothing was found")
			}
			if !domain.IsWarning(err) {
				t.Errorf("ExecDiscover error = %v, want a downgradeable warning", err)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %q, want it to explain with %q", err, tt.want)
			}
			if got := readConfig(t, deps.ConfigPath); got != "" {
				t.Errorf("config file = %q, want nothing written", got)
			}
		})
	}
}

// TestExecDiscoverSkipsWhatAProjectCovers checks that covered directories are
// not proposed again.
func TestExecDiscoverSkipsWhatAProjectCovers(t *testing.T) {
	t.Parallel()

	root := tree(t, "known/a", "known/b")
	deps := clitest.New(t, fakeGit{})
	deps.ConfigPath = configFile(t, "")
	deps.Projects = domain.ProjectListKeyed{
		"known": {Name: "known", Path: filepath.Join(root, "known")},
	}

	err := ExecDiscover(Options{}, []string{root}, deps.RuntimeCLI)
	if err == nil {
		t.Fatal("ExecDiscover error = nil, want a warning that everything is covered")
	}
	if want := "already covered"; !strings.Contains(err.Error(), want) {
		t.Errorf("error = %q, want it to say %q", err, want)
	}
}

// TestExecDiscoverMinimum checks that --min reaches the scan.
func TestExecDiscoverMinimum(t *testing.T) {
	t.Parallel()

	t.Run("default files a lone repository", func(t *testing.T) {
		t.Parallel()

		root := tree(t, "solo/only-one")
		deps := clitest.New(t, fakeGit{})
		deps.ConfigPath = configFile(t, "")

		if err := ExecDiscover(Options{}, []string{root}, deps.RuntimeCLI); err != nil {
			t.Fatalf("ExecDiscover error = %v, want nil", err)
		}
		if config := readConfig(t, deps.ConfigPath); !strings.Contains(config, "solo:") {
			t.Errorf("config is missing the lone repository's directory:\n%s", config)
		}
		if want := "1 repository"; !strings.Contains(deps.Result(), want) {
			t.Errorf("Result Output = %q, want the singular %q", deps.Result(), want)
		}
	})

	t.Run("min 2 excludes it", func(t *testing.T) {
		t.Parallel()

		root := tree(t, "solo/only-one")
		deps := clitest.New(t, fakeGit{})
		deps.ConfigPath = configFile(t, "")

		if err := ExecDiscover(Options{Min: 2}, []string{root}, deps.RuntimeCLI); err == nil {
			t.Fatal("ExecDiscover error = nil, want a warning that nothing cleared the minimum")
		}
		if got := readConfig(t, deps.ConfigPath); got != "" {
			t.Errorf("config file = %q, want nothing written", got)
		}
	})
}

// TestExecDiscoverRejectsANonDirectory checks that a non-directory path fails.
func TestExecDiscoverRejectsANonDirectory(t *testing.T) {
	t.Parallel()

	deps := clitest.New(t, fakeGit{})
	deps.ConfigPath = configFile(t, "")

	err := ExecDiscover(Options{}, []string{filepath.Join(t.TempDir(), "nope")}, deps.RuntimeCLI)
	if err == nil {
		t.Fatal("ExecDiscover error = nil, want a path that is no directory to be refused")
	}
	if domain.IsWarning(err) {
		t.Errorf("ExecDiscover error = %v, want a real error rather than a warning", err)
	}
}

// TestExecDiscoverRendersHomeRelativePaths checks that paths under home are
// written with ~.
func TestExecDiscoverRendersHomeRelativePaths(t *testing.T) {
	t.Parallel()

	root := tree(t, "work/a", "work/b")
	deps := clitest.New(t, fakeGit{})
	deps.ConfigPath = configFile(t, "")
	// Use the fixture tree as home.
	deps.HomeDir = root

	if err := ExecDiscover(Options{}, []string{root}, deps.RuntimeCLI); err != nil {
		t.Fatalf("ExecDiscover error = %v, want nil", err)
	}
	if want := "path: ~/work"; !strings.Contains(readConfig(t, deps.ConfigPath), want) {
		t.Errorf("config = %q, want the home-relative %q",
			readConfig(t, deps.ConfigPath), want)
	}
}

// TestExecDiscoverSkipsANameOnlyTheFileKnows checks that a name taken in the
// file but not in the loaded projects is skipped.
func TestExecDiscoverSkipsANameOnlyTheFileKnows(t *testing.T) {
	t.Parallel()

	root := tree(t, "work/a", "work/b")
	deps := clitest.New(t, fakeGit{})
	deps.ConfigPath = configFile(t, "work:\n  path: ~/elsewhere\n")
	// Empty: the loader never saw this project.
	deps.Projects = domain.ProjectListKeyed{}

	err := ExecDiscover(Options{}, []string{root}, deps.RuntimeCLI)
	if err == nil {
		t.Fatal("ExecDiscover error = nil, want a warning that nothing was added")
	}
	if !domain.IsWarning(err) {
		t.Errorf("ExecDiscover error = %v, want a downgradeable warning", err)
	}

	config := readConfig(t, deps.ConfigPath)
	if n := strings.Count(config, "work:"); n != 1 {
		t.Errorf("config declares `work:` %d times, want exactly 1:\n%s", n, config)
	}
	if strings.Contains(config, filepath.Join(root, "work")) {
		t.Errorf("config gained a second project under the same name:\n%s", config)
	}
	if want := "already in the config file"; !strings.Contains(deps.Diagnostic(), want) {
		t.Errorf("Diagnostic Output = %q, want it to say %q", deps.Diagnostic(), want)
	}
}

// TestExecDiscoverWritesOnlyUntakenGroups checks that a name taken in the
// loaded projects but not in the file is skipped.
func TestExecDiscoverWritesOnlyUntakenGroups(t *testing.T) {
	t.Parallel()

	root := tree(t, "work/a", "work/b", "oss/c", "oss/d")
	deps := clitest.New(t, fakeGit{})
	// Empty file: only the loaded projects hold the name.
	deps.ConfigPath = configFile(t, "")
	deps.Projects = domain.ProjectListKeyed{
		"work": {Name: "work", Path: "~/elsewhere"},
	}

	if err := ExecDiscover(Options{}, []string{root}, deps.RuntimeCLI); err != nil {
		t.Fatalf("ExecDiscover error = %v, want nil", err)
	}

	config := readConfig(t, deps.ConfigPath)
	if strings.Contains(config, "work:") {
		t.Errorf("config holds a project the loaded list already names:\n%s", config)
	}
	if !strings.Contains(config, "oss:") {
		t.Errorf("config is missing the group whose name was free:\n%s", config)
	}
}

// TestExecDiscoverMixedLayoutMakesEveryDirectoryAProject checks that each
// directory becomes a project of only its direct repositories.
func TestExecDiscoverMixedLayoutMakesEveryDirectoryAProject(t *testing.T) {
	t.Parallel()

	// Named root, so the project is `src`.
	root := filepath.Join(tree(t), "src")
	repos(t, root,
		"project1/repo1", "project1/repo2",
		"repo3",
		"project2/repo4", "project2/repo5",
		"repo6",
	)
	deps := clitest.New(t, fakeGit{})
	deps.ConfigPath = configFile(t, "")

	if err := ExecDiscover(Options{}, []string{root}, deps.RuntimeCLI); err != nil {
		t.Fatalf("ExecDiscover error = %v, want nil", err)
	}

	// The root lists only its own two repositories.
	want := fmt.Sprintf(`src:
  path: %s
  repos:
    - dir: repo3
      src: git@example.com:o/repo3.git
    - dir: repo6
      src: git@example.com:o/repo6.git
project1:
  path: %s
  repos:
    - dir: repo1
      src: git@example.com:o/repo1.git
    - dir: repo2
      src: git@example.com:o/repo2.git
project2:
  path: %s
  repos:
    - dir: repo4
      src: git@example.com:o/repo4.git
    - dir: repo5
      src: git@example.com:o/repo5.git
`,
		root,
		filepath.Join(root, "project1"), filepath.Join(root, "project2"))

	if got := readConfig(t, deps.ConfigPath); got != want {
		t.Errorf("config =\n%s\nwant:\n%s", got, want)
	}
}

// TestExecDiscoverSaysWhatItFoundBelowTheMinimum checks that repositories
// below --min are reported, not "nothing found".
func TestExecDiscoverSaysWhatItFoundBelowTheMinimum(t *testing.T) {
	t.Parallel()

	root := tree(t, "a/one", "b/one")
	deps := clitest.New(t, fakeGit{})
	deps.ConfigPath = configFile(t, "")

	err := ExecDiscover(Options{Min: 3}, []string{root}, deps.RuntimeCLI)
	if err == nil {
		t.Fatal("ExecDiscover error = nil, want a warning that nothing cleared the minimum")
	}
	if want := "3 or more repositories"; !strings.Contains(err.Error(), want) {
		t.Errorf("error = %q, want it to name the minimum with %q", err, want)
	}
	if strings.Contains(err.Error(), "no git repositories found") {
		t.Errorf("error = %q, want it not to claim the scan found nothing", err)
	}
}

// TestExecDiscoverRequiresAPath checks that the path argument is required.
func TestExecDiscoverRequiresAPath(t *testing.T) {
	t.Parallel()

	deps := clitest.New(t, fakeGit{})
	deps.ConfigPath = configFile(t, "")

	err := ExecDiscover(Options{}, nil, deps.RuntimeCLI)
	if err == nil {
		t.Fatal("ExecDiscover(no args) = nil, want the missing path to be refused")
	}
	if got := readConfig(t, deps.ConfigPath); got != "" {
		t.Errorf("config file = %q, want nothing written", got)
	}
}

// TestExecDiscoverAppendsToAProjectAtTheSamePath checks that new clones are
// appended to a project listing its repositories.
func TestExecDiscoverAppendsToAProjectAtTheSamePath(t *testing.T) {
	t.Parallel()

	root := tree(t, "work/a", "work/fresh")
	workPath := filepath.Join(root, "work")
	deps := clitest.New(t, fakeGit{})
	deps.ConfigPath = configFile(t, fmt.Sprintf(
		"work:\n  path: %s\n  repos:\n    - dir: a\n      src: git@example.com:o/a.git\n",
		workPath))
	deps.Projects = domain.ProjectListKeyed{
		"work": {Name: "work", Path: workPath, Repos: []domain.Repository{
			{Name: "a", AbsPath: filepath.Join(workPath, "a")},
		}},
	}

	if err := ExecDiscover(Options{}, []string{root}, deps.RuntimeCLI); err != nil {
		t.Fatalf("ExecDiscover error = %v, want nil", err)
	}

	config := readConfig(t, deps.ConfigPath)
	if n := strings.Count(config, "work:"); n != 1 {
		t.Errorf("config declares `work:` %d times, want the existing project reused:\n%s",
			n, config)
	}
	if !strings.Contains(config, "- dir: fresh") {
		t.Errorf("config is missing the repository cloned since the last run:\n%s", config)
	}
	if !strings.Contains(config, "src: git@example.com:o/fresh.git") {
		t.Errorf("config is missing the appended repository's remote:\n%s", config)
	}
}

// TestExecDiscoverWritesTheRemoteOfEachRepository checks that `src:` is
// written when a clone has a remote.
func TestExecDiscoverWritesTheRemoteOfEachRepository(t *testing.T) {
	t.Parallel()

	root := tree(t, "work/api", "work/no-remote")
	deps := clitest.New(t, fakeGit{})
	deps.ConfigPath = configFile(t, "")

	if err := ExecDiscover(Options{}, []string{root}, deps.RuntimeCLI); err != nil {
		t.Fatalf("ExecDiscover error = %v, want nil", err)
	}

	config := readConfig(t, deps.ConfigPath)
	want := "    - dir: api\n      src: git@example.com:o/api.git\n    - dir: no-remote\n"
	if !strings.Contains(config, want) {
		t.Errorf("config =\n%s\nwant it to contain:\n%s", config, want)
	}
}

// TestExecDiscoverNamesDuplicateDirectoriesApart checks that same-named
// directories get distinct project names.
func TestExecDiscoverNamesDuplicateDirectoriesApart(t *testing.T) {
	t.Parallel()

	root := tree(t, "personal/archive/a", "work/archive/b")
	deps := clitest.New(t, fakeGit{})
	deps.ConfigPath = configFile(t, "")

	if err := ExecDiscover(Options{}, []string{root}, deps.RuntimeCLI); err != nil {
		t.Fatalf("ExecDiscover error = %v, want nil", err)
	}

	config := readConfig(t, deps.ConfigPath)
	for _, want := range []string{"archive:", "work-archive:"} {
		if !strings.Contains(config, want) {
			t.Errorf("config is missing %q:\n%s", want, config)
		}
	}
	if n := strings.Count(config, "\narchive:"); n > 1 {
		t.Errorf("config declares `archive:` more than once:\n%s", config)
	}
}

// TestExecDiscoverDoesNotDuplicateADirectoryUnderAProjectPath checks that
// clones under a project's path join that project.
func TestExecDiscoverDoesNotDuplicateADirectoryUnderAProjectPath(t *testing.T) {
	t.Parallel()

	root := tree(t, "github/rafi/one", "github/old")
	ghPath := filepath.Join(root, "github")
	deps := clitest.New(t, fakeGit{})
	deps.ConfigPath = configFile(t, fmt.Sprintf(
		"gh:\n  path: %s\n  repos:\n    - dir: old\n      src: git@example.com:o/old.git\n",
		ghPath))
	deps.Projects = domain.ProjectListKeyed{
		"gh": {Name: "gh", Path: ghPath, Repos: []domain.Repository{
			{Name: "old", AbsPath: filepath.Join(ghPath, "old")},
		}},
	}

	if err := ExecDiscover(Options{}, []string{root}, deps.RuntimeCLI); err != nil {
		t.Fatalf("ExecDiscover error = %v, want nil", err)
	}

	config := readConfig(t, deps.ConfigPath)
	if strings.Contains(config, "github:") {
		t.Errorf("config gained a project over ground `gh` already covers:\n%s", config)
	}
	// Relative to `gh`'s path.
	if !strings.Contains(config, "- dir: rafi/one") {
		t.Errorf("config is missing the new repository under `gh`:\n%s", config)
	}
}

// TestExecDiscoverLeavesASourceProjectsPathAlone checks that nothing is
// written under a project with a source.
func TestExecDiscoverLeavesASourceProjectsPathAlone(t *testing.T) {
	t.Parallel()

	root := tree(t, "github/rafi/one", "github/rafi/two", "elsewhere/three")
	ghPath := filepath.Join(root, "github")
	before := fmt.Sprintf("gh:\n  source:\n    type: github\n    search: user:rafi\n  path: %s\n",
		ghPath)
	deps := clitest.New(t, fakeGit{})
	deps.ConfigPath = configFile(t, before)
	deps.Projects = domain.ProjectListKeyed{
		"gh": {
			Name:   "gh",
			Path:   ghPath,
			Source: &domain.ProviderSource{Type: "github", Search: "user:rafi"},
			Repos:  []domain.Repository{{Name: "one", AbsPath: filepath.Join(ghPath, "rafi", "one")}},
		},
	}

	if err := ExecDiscover(Options{}, []string{root}, deps.RuntimeCLI); err != nil {
		t.Fatalf("ExecDiscover error = %v, want nil", err)
	}

	config := readConfig(t, deps.ConfigPath)
	if !strings.HasPrefix(config, before) {
		t.Errorf("config = %q, want the `gh` project untouched", config)
	}
	for _, unwanted := range []string{"github:", "rafi:", "gh-", "dir: rafi"} {
		if strings.Contains(config, unwanted) {
			t.Errorf("config gained %q over ground `gh` already covers:\n%s", unwanted, config)
		}
	}
	if !strings.Contains(config, "elsewhere:") {
		t.Errorf("config is missing the directory no project covers:\n%s", config)
	}
}

// TestExecDiscoverNeverWritesAnEscapingDir checks that a `dir:` outside the
// project path is written in full.
func TestExecDiscoverNeverWritesAnEscapingDir(t *testing.T) {
	t.Parallel()

	root := tree(t, "code/own/a", "elsewhere/far", "elsewhere/fresh")
	deps := clitest.New(t, fakeGit{})
	deps.ConfigPath = configFile(t, fmt.Sprintf(
		"dotfiles:\n  path: %s\n  repos:\n    - dir: %s\n",
		filepath.Join(root, "code"), filepath.Join(root, "elsewhere", "far")))
	deps.Projects = domain.ProjectListKeyed{
		"dotfiles": {Name: "dotfiles", Path: filepath.Join(root, "code"),
			Repos: []domain.Repository{
				{Name: "far", AbsPath: filepath.Join(root, "elsewhere", "far")},
			}},
	}

	if err := ExecDiscover(Options{}, []string{root}, deps.RuntimeCLI); err != nil {
		t.Fatalf("ExecDiscover error = %v, want nil", err)
	}

	config := readConfig(t, deps.ConfigPath)
	if strings.Contains(config, "../") {
		t.Errorf("config holds a `dir:` climbing out of its project:\n%s", config)
	}
	// Under the project path, still relative.
	if !strings.Contains(config, "- dir: own/a") {
		t.Errorf("config lost the relative dir of a repository inside the project:\n%s", config)
	}
	if !strings.Contains(config, "- dir: "+filepath.Join(root, "elsewhere", "fresh")) {
		t.Errorf("config is missing the outside repository named in full:\n%s", config)
	}
}

// loadedDeps builds dependencies with Projects parsed from content, unresolved
// as the CLI passes them.
func loadedDeps(t *testing.T, content string) *clitest.Deps {
	t.Helper()
	deps := clitest.New(t, fakeGit{})
	deps.ConfigPath = configFile(t, content)
	var file config.File
	if err := config.NewConfigFromFile(deps.ConfigPath, &file); err != nil {
		t.Fatalf("load config: %v", err)
	}
	deps.Projects = file.Projects
	return deps
}

// TestExecDiscoverRerunAppendsNewClones checks that a rerun appends only new
// clones.
func TestExecDiscoverRerunAppendsNewClones(t *testing.T) {
	t.Parallel()

	root := tree(t, "work/a")
	workPath := filepath.Join(root, "work")
	deps := loadedDeps(t, fmt.Sprintf("work:\n  path: %s\n  repos:\n    - dir: a\n", workPath))
	repos(t, root, "work/fresh")

	if err := ExecDiscover(Options{}, []string{root}, deps.RuntimeCLI); err != nil {
		t.Fatalf("ExecDiscover error = %v, want nil", err)
	}

	config := readConfig(t, deps.ConfigPath)
	if n := strings.Count(config, "work:"); n != 1 {
		t.Errorf("config declares `work:` %d times, want the existing project reused:\n%s", n, config)
	}
	if n := strings.Count(config, "dir: a"); n != 1 {
		t.Errorf("config lists `a` %d times, want it once:\n%s", n, config)
	}
	if !strings.Contains(config, "- dir: fresh") {
		t.Errorf("config is missing the repository cloned since the last run:\n%s", config)
	}
}

// TestExecDiscoverSkipsARepoListedByAbsoluteDir checks that a repository
// listed by absolute `dir:` is not written again.
func TestExecDiscoverSkipsARepoListedByAbsoluteDir(t *testing.T) {
	t.Parallel()

	root := tree(t, "proj/a", "proj/b")
	deps := loadedDeps(t, fmt.Sprintf("tools:\n  repos:\n    - dir: %s\n",
		filepath.Join(root, "proj", "a")))

	if err := ExecDiscover(Options{}, []string{root}, deps.RuntimeCLI); err != nil {
		t.Fatalf("ExecDiscover error = %v, want nil", err)
	}

	config := readConfig(t, deps.ConfigPath)
	if strings.Contains(config, "- dir: a") {
		t.Errorf("config lists `a` under a second project:\n%s", config)
	}
}

// TestExecDiscoverAppendsToASubProject checks that a clone under a
// Sub-project is appended to it.
func TestExecDiscoverAppendsToASubProject(t *testing.T) {
	t.Parallel()

	root := tree(t, "org/team/a")
	deps := loadedDeps(t, fmt.Sprintf(
		"org:\n  path: %s\n  repos:\n    - dir: other\n"+
			"  subprojects:\n    - name: team\n      repos:\n        - dir: a\n",
		filepath.Join(root, "org")))
	repos(t, root, "org/team/fresh")

	if err := ExecDiscover(Options{}, []string{root}, deps.RuntimeCLI); err != nil {
		t.Fatalf("ExecDiscover error = %v, want nil", err)
	}

	config := readConfig(t, deps.ConfigPath)
	if !strings.Contains(config, "        - dir: a\n        - dir: fresh") {
		t.Errorf("config is missing `fresh` under the sub-project:\n%s", config)
	}
	if strings.Contains(config, "team:") {
		t.Errorf("config gained a top-level project for the sub-project's directory:\n%s", config)
	}
}
