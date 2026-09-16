package discover

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/infra/git"
	coreruntime "github.com/rafi/gits/internal/runtime"
)

// fakeGit treats only directories containing .git as repositories.
type fakeGit struct {
	git.Client
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

// scanDeps is the runtime a scan needs: a context and a git client.
func scanDeps(t *testing.T) coreruntime.Runtime {
	t.Helper()
	return coreruntime.Runtime{Ctx: t.Context(), Git: fakeGit{}}
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

// names returns the group names in the order the scan returned them.
func names(result Result) []string {
	out := make([]string, 0, len(result.Groups))
	for _, g := range result.Groups {
		out = append(out, g.Name)
	}
	return out
}

// reposOf returns the base names of a group's repositories.
func reposOf(result Result, name string) []string {
	for _, g := range result.Groups {
		if g.Name != name {
			continue
		}
		out := make([]string, 0, len(g.Repos))
		for _, repo := range g.Repos {
			out = append(out, filepath.Base(repo.Path))
		}
		return out
	}
	return nil
}

// TestScanMixedLayout checks that each directory becomes a Group of only its
// direct repositories.
func TestScanMixedLayout(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	repos(t, root,
		"project1/repo1", "project1/repo2",
		"repo3",
		"project2/repo4", "project2/repo5",
		"repo6",
	)

	result, err := Scan(root, scanDeps(t), Options{})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	got := names(result)
	want := []string{filepath.Base(root), "project1", "project2"}
	if !slices.Equal(got, want) {
		t.Fatalf("group names = %v, want %v", got, want)
	}
	for name, wantRepos := range map[string][]string{
		filepath.Base(root): {"repo3", "repo6"},
		"project1":          {"repo1", "repo2"},
		"project2":          {"repo4", "repo5"},
	} {
		if gotRepos := reposOf(result, name); !slices.Equal(gotRepos, wantRepos) {
			t.Errorf("project %q holds %v, want %v", name, gotRepos, wantRepos)
		}
	}
	if result.Total() != 6 {
		t.Errorf("Total() = %d, want every repository found, 6", result.Total())
	}
}

// TestScanGroupsByImmediateParent checks that repositories group by their
// immediate parent at any depth.
func TestScanGroupsByImmediateParent(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	repos(t, root,
		"src/github.com/acme/api", "src/github.com/acme/web",
		"very/deeply/nested/down/e",
	)

	result, err := Scan(root, scanDeps(t), Options{})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if got, want := names(result), []string{"acme", "down"}; !slices.Equal(got, want) {
		t.Fatalf("group names = %v, want %v", got, want)
	}
	for _, g := range result.Groups {
		if filepath.Base(g.Path) != g.Name {
			t.Errorf("group %q sits at %q, want a project named after its own directory",
				g.Name, g.Path)
		}
	}
}

// TestScanMinimum checks that directories below Min become no Group.
func TestScanMinimum(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	repos(t, root, "work/a", "work/b", "work/c", "oss/d", "oss/e", "solo/f")

	t.Run("default files even a lone repository", func(t *testing.T) {
		t.Parallel()

		result, err := Scan(root, scanDeps(t), Options{})
		if err != nil {
			t.Fatalf("Scan: %v", err)
		}
		if got, want := names(result), []string{"oss", "solo", "work"}; !slices.Equal(got, want) {
			t.Errorf("group names = %v, want %v", got, want)
		}
	})

	t.Run("min 3 keeps only the largest", func(t *testing.T) {
		t.Parallel()

		result, err := Scan(root, scanDeps(t), Options{Min: 3})
		if err != nil {
			t.Fatalf("Scan: %v", err)
		}
		if got, want := names(result), []string{"work"}; !slices.Equal(got, want) {
			t.Errorf("group names = %v, want %v", got, want)
		}
	})
}

// TestScanSkipsWhatAProjectAlreadyCovers checks that covered repositories are
// not proposed again.
func TestScanSkipsWhatAProjectAlreadyCovers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		existing func(root string) domain.ProjectListKeyed
	}{
		{
			name: "a project path",
			existing: func(root string) domain.ProjectListKeyed {
				return domain.ProjectListKeyed{
					"known": {Name: "known", Path: filepath.Join(root, "known")},
				}
			},
		},
		{
			name: "repositories listed by hand",
			existing: func(root string) domain.ProjectListKeyed {
				return domain.ProjectListKeyed{
					"dotfiles": {Name: "dotfiles", Repos: []domain.Repository{
						{Name: "a", AbsPath: filepath.Join(root, "known", "a")},
						{Name: "b", AbsPath: filepath.Join(root, "known", "b")},
					}},
				}
			},
		},
		{
			name: "a sub-project's path",
			existing: func(root string) domain.ProjectListKeyed {
				return domain.ProjectListKeyed{
					"parent": {Name: "parent", SubProjects: []domain.Project{
						{Name: "child", Path: filepath.Join(root, "known")},
					}},
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			repos(t, root, "known/a", "known/b", "fresh/c")

			result, err := Scan(root, scanDeps(t), Options{Existing: tt.existing(root)})
			if err != nil {
				t.Fatalf("Scan: %v", err)
			}
			if got, want := names(result), []string{"fresh"}; !slices.Equal(got, want) {
				t.Fatalf("group names = %v, want %v", got, want)
			}
			if result.Covered != 2 {
				t.Errorf("Covered = %d, want the 2 repositories already accounted for", result.Covered)
			}
		})
	}
}

// TestScanReportsTakenNameWithoutWriting checks that a taken name is marked
// and kept out of Writable.
func TestScanReportsTakenNameWithoutWriting(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	repos(t, root, "work/a", "oss/b")

	// A `work` project elsewhere.
	existing := domain.ProjectListKeyed{
		"work": {Name: "work", Path: filepath.Join(t.TempDir(), "elsewhere")},
	}
	result, err := Scan(root, scanDeps(t), Options{Existing: existing})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(result.Groups) != 2 {
		t.Fatalf("group names = %v, want both directories reported", names(result))
	}
	var work Group
	for _, g := range result.Groups {
		if g.Name == "work" {
			work = g
		}
	}
	if !work.Taken {
		t.Error("group `work` Taken = false, want the name reported as already a project's")
	}
	writable := result.Writable()
	if len(writable) != 1 || writable[0].Name != "oss" {
		t.Errorf("Writable() = %v, want only the group whose name is free",
			names(Result{Groups: writable}))
	}
}

// TestScanDoesNotDescendIntoRepositories checks that nested clones are
// ignored.
func TestScanDoesNotDescendIntoRepositories(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	repos(t, root, "group/a", "group/b")
	repos(t, root, "group/a/vendor/embedded")

	result, err := Scan(root, scanDeps(t), Options{})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if got, want := names(result), []string{"group"}; !slices.Equal(got, want) {
		t.Fatalf("group names = %v, want %v", got, want)
	}
	if got := reposOf(result, "group"); !slices.Equal(got, []string{"a", "b"}) {
		t.Errorf("group holds %v, want [a b]: the embedded one is inside a clone", got)
	}
}

// TestScanRejectsNonDirectory checks that a non-directory root fails.
func TestScanRejectsNonDirectory(t *testing.T) {
	t.Parallel()

	file := filepath.Join(t.TempDir(), "a-file")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	if _, err := Scan(file, scanDeps(t), Options{}); err == nil {
		t.Error("Scan(file) = nil, want a path that is no directory to be refused")
	}
	if _, err := Scan(filepath.Join(t.TempDir(), "nope"), scanDeps(t), Options{}); err == nil {
		t.Error("Scan(missing) = nil, want a path that does not exist to be refused")
	}
}

// TestScanFindsNothingInAnEmptyTree checks that an empty tree is not an
// error.
func TestScanFindsNothingInAnEmptyTree(t *testing.T) {
	t.Parallel()

	result, err := Scan(t.TempDir(), scanDeps(t), Options{})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(result.Groups) != 0 || result.Total() != 0 {
		t.Errorf("Scan of an empty tree = %+v, want nothing found", result)
	}
}

// TestScanLooksInsideAListedProjectsPath checks that new clones under a
// project listing its repositories are found.
func TestScanLooksInsideAListedProjectsPath(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	repos(t, root, "work/a", "work/b", "work/fresh")

	existing := domain.ProjectListKeyed{
		"work": {Name: "work", Path: filepath.Join(root, "work"), Repos: []domain.Repository{
			{Name: "a", AbsPath: filepath.Join(root, "work", "a")},
			{Name: "b", AbsPath: filepath.Join(root, "work", "b")},
		}},
	}
	result, err := Scan(root, scanDeps(t), Options{Existing: existing})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if got, want := names(result), []string{"work"}; !slices.Equal(got, want) {
		t.Fatalf("group names = %v, want %v", got, want)
	}
	if got := reposOf(result, "work"); !slices.Equal(got, []string{"fresh"}) {
		t.Errorf("group holds %v, want only the repository added since, [fresh]", got)
	}
	group := result.Groups[0]
	if group.Existing != "work" {
		t.Errorf("Existing = %q, want the project already at that path, \"work\"", group.Existing)
	}
	if group.Taken {
		t.Error("Taken = true, want a group appended to its own project to be writable")
	}
	if len(result.Writable()) != 1 {
		t.Error("Writable() dropped the group, want the new repository recorded")
	}
}

// TestScanKeepsSkippingADiscoveredProjectsPath checks that nothing under a
// searched project path is proposed.
func TestScanKeepsSkippingADiscoveredProjectsPath(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	repos(t, root, "work/a", "work/fresh")

	for name, existing := range map[string]domain.ProjectListKeyed{
		"a path and no repos": {
			"work": {Name: "work", Path: filepath.Join(root, "work")},
		},
		"an explicit filesystem source": {
			"work": {
				Name:   "work",
				Path:   filepath.Join(root, "work"),
				Source: &domain.ProviderSource{Type: "filesystem"},
				Repos:  []domain.Repository{{Name: "a"}},
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			result, err := Scan(root, scanDeps(t), Options{Existing: existing})
			if err != nil {
				t.Fatalf("Scan: %v", err)
			}
			if len(result.Groups) != 0 {
				t.Errorf("group names = %v, want nothing: the project discovers that tree itself",
					names(result))
			}
			if result.Covered != 2 {
				t.Errorf("Covered = %d, want both repositories accounted for", result.Covered)
			}
		})
	}
}

// TestScanNamesDuplicateDirectoriesApart checks that same-named directories
// get distinct names.
func TestScanNamesDuplicateDirectoriesApart(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	repos(t, root, "personal/archive/a", "work/archive/b", "work/current/c")

	result, err := Scan(root, scanDeps(t), Options{})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	// Ordered by path: `personal/archive` keeps the plain name.
	want := []string{"archive", "work-archive", "current"}
	if got := names(result); !slices.Equal(got, want) {
		t.Fatalf("group names = %v, want %v", got, want)
	}
	if got := reposOf(result, "work-archive"); !slices.Equal(got, []string{"b"}) {
		t.Errorf("work-archive holds %v, want [b]", got)
	}
}

// TestScanReadsEachRepositorysRemote checks that remotes are read and clones
// without one are kept.
func TestScanReadsEachRepositorysRemote(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	repos(t, root, "work/api", "work/no-remote")

	result, err := Scan(root, scanDeps(t), Options{})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(result.Groups) != 1 {
		t.Fatalf("group names = %v, want one group", names(result))
	}
	want := map[string]string{
		"api":       "git@example.com:o/api.git",
		"no-remote": "",
	}
	for _, repo := range result.Groups[0].Repos {
		if got := repo.Src; got != want[filepath.Base(repo.Path)] {
			t.Errorf("%s Src = %q, want %q", filepath.Base(repo.Path), got,
				want[filepath.Base(repo.Path)])
		}
	}
}

// TestScanClaimsDirectoriesUnderAProjectPath checks that directories under a
// project path are assigned to that project.
func TestScanClaimsDirectoriesUnderAProjectPath(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	repos(t, root, "github/rafi/one", "github/rafi/two", "elsewhere/three")

	existing := domain.ProjectListKeyed{
		"gh": {Name: "gh", Path: filepath.Join(root, "github"), Repos: []domain.Repository{
			{Name: "old", AbsPath: filepath.Join(root, "github", "old")},
		}},
	}
	result, err := Scan(root, scanDeps(t), Options{Existing: existing})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if got, want := names(result), []string{"elsewhere", "gh"}; !slices.Equal(got, want) {
		t.Fatalf("group names = %v, want %v: the nested directory is the project's, not a new one",
			got, want)
	}
	for _, g := range result.Groups {
		if g.Name != "gh" {
			continue
		}
		if g.Existing != "gh" {
			t.Errorf("Existing = %q, want the project whose path holds it, \"gh\"", g.Existing)
		}
		if g.ExistingPath != filepath.Join(root, "github") {
			t.Errorf("ExistingPath = %q, want the project's own path", g.ExistingPath)
		}
	}
}

// TestScanOwnerIsTheDeepestProjectPath checks that the closest project path
// owns a directory.
func TestScanOwnerIsTheDeepestProjectPath(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	repos(t, root, "code/github/rafi/one")

	existing := domain.ProjectListKeyed{
		"outer": {Name: "outer", Path: filepath.Join(root, "code"), Repos: []domain.Repository{
			{Name: "x", AbsPath: filepath.Join(root, "code", "x")},
		}},
		"inner": {
			Name: "inner",
			Path: filepath.Join(root, "code", "github"),
			Repos: []domain.Repository{
				{Name: "y", AbsPath: filepath.Join(root, "code", "github", "y")},
			},
		},
	}
	result, err := Scan(root, scanDeps(t), Options{Existing: existing})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if got, want := names(result), []string{"inner"}; !slices.Equal(got, want) {
		t.Fatalf("group names = %v, want %v", got, want)
	}
}

// TestScanClaimsTheDirectoryAListedProjectsReposLiveIn checks that a project
// without a path owns the directory its repositories live in.
func TestScanClaimsTheDirectoryAListedProjectsReposLiveIn(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	repos(t, root, "work/a", "work/fresh")

	existing := domain.ProjectListKeyed{
		"work-by-hand": {Name: "work-by-hand", Repos: []domain.Repository{
			{Name: "a", AbsPath: filepath.Join(root, "work", "a")},
		}},
	}
	result, err := Scan(root, scanDeps(t), Options{Existing: existing})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if got, want := names(result), []string{"work-by-hand"}; !slices.Equal(got, want) {
		t.Fatalf("group names = %v, want %v", got, want)
	}
	if got := reposOf(result, "work-by-hand"); !slices.Equal(got, []string{"fresh"}) {
		t.Errorf("group holds %v, want only the repository added since, [fresh]", got)
	}
	// No project path to be relative to.
	if got := result.Groups[0].ExistingPath; got != "" {
		t.Errorf("ExistingPath = %q, want empty: the project declares no path", got)
	}
}

// TestScanSkipsTheTreeOfARemoteSourceProject checks that nothing under a
// forge-sourced project's path is proposed.
func TestScanSkipsTheTreeOfARemoteSourceProject(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	repos(t, root, "github/rafi/one", "github/rafi/two", "elsewhere/three")

	existing := domain.ProjectListKeyed{
		"gh": {
			Name:   "gh",
			Path:   filepath.Join(root, "github"),
			Source: &domain.ProviderSource{Type: "github", Search: "user:rafi"},
			Repos:  []domain.Repository{{Name: "one"}},
		},
	}
	result, err := Scan(root, scanDeps(t), Options{Existing: existing})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if got, want := names(result), []string{"elsewhere"}; !slices.Equal(got, want) {
		t.Fatalf("group names = %v, want %v: the forge project owns its whole path", got, want)
	}
	if result.Covered != 2 {
		t.Errorf("Covered = %d, want the 2 repositories under the forge project's path",
			result.Covered)
	}
}
