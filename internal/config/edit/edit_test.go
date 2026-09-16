package edit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSaveRoundTrip proves save writes next to the config file (a rename
// across filesystems would fail with EXDEV), preserves the original file
// mode, and leaves no stray temp files behind.
func TestSaveRoundTrip(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := "myproj:\n  repos:\n    - dir: ~/code/x\n      src: git@x:a/x.git\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatalf("chmod: %v", err)
	}

	doc, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if err := doc.Save(path); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if !strings.Contains(string(got), "src: git@x:a/x.git") {
		t.Errorf("saved config lost content:\n%s", got)
	}

	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if fi.Mode().Perm() != 0o640 {
		t.Errorf("mode = %o, want 640 (original mode must survive save)", fi.Mode().Perm())
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "config.yaml" {
		names := []string{}
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("stray files after save: %v", names)
	}
}

// TestAddRepoPreservesFile proves that adding a repository changes nothing in
// the config file but the lines it appends: comments, blank lines, quoting,
// and the indentation style of each list all read back as they were written.
func TestAddRepoPreservesFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		project string
		in      string
		want    string
	}{
		{
			name:    "block list keeps comments, their spacing, blank lines and quotes",
			project: "alpha",
			in: `# top comment

# about alpha
alpha:
  path: ~/a   # where it lives
  repos:

    - dir: ~/x # trailing
      src: "git@x:a/x.git"

    # about z
    - dir: ~/z
      src: 'git@x:a/z.git'

# footer

beta:
  repos: []
`,
			want: `# top comment

# about alpha
alpha:
  path: ~/a   # where it lives
  repos:

    - dir: ~/x # trailing
      src: "git@x:a/x.git"

    # about z
    - dir: ~/z
      src: 'git@x:a/z.git'
    - dir: ~/new
      src: git@h:o/new.git

# footer

beta:
  repos: []
`,
		},
		{
			name:    "block list indented flush with its key",
			project: "alpha",
			in: `alpha:
  repos:
  - dir: ~/x
    src: git@x:a/x.git
beta:
  path: ~/b
`,
			want: `alpha:
  repos:
  - dir: ~/x
    src: git@x:a/x.git
  - dir: ~/new
    src: git@h:o/new.git
beta:
  path: ~/b
`,
		},
		{
			name:    "four space indentation",
			project: "alpha",
			in: `alpha:
    repos:
        - dir: ~/x
          src: git@x:a/x.git
`,
			want: `alpha:
    repos:
        - dir: ~/x
          src: git@x:a/x.git
        - dir: ~/new
          src: git@h:o/new.git
`,
		},
		{
			name:    "empty flow list becomes a block list",
			project: "alpha",
			in: `alpha:
  repos: []  # none yet
beta:
  path: ~/b
`,
			want: `alpha:
  repos: # none yet
    - dir: ~/new
      src: git@h:o/new.git
beta:
  path: ~/b
`,
		},
		{
			name:    "flow list gets a flow item",
			project: "alpha",
			in: `alpha:
  repos: [{dir: ~/x, src: git@x:a/x.git}]
`,
			want: `alpha:
  repos: [{dir: ~/x, src: git@x:a/x.git}, {dir: ~/new, src: "git@h:o/new.git"}]
`,
		},
		{
			name:    "null repos becomes a block list",
			project: "alpha",
			in: `alpha:
  desc: hand listed
  repos:
beta:
  path: ~/b
`,
			want: `alpha:
  desc: hand listed
  repos:
    - dir: ~/new
      src: git@h:o/new.git
beta:
  path: ~/b
`,
		},
		{
			name:    "missing repos key is appended to the project",
			project: "alpha",
			in: `alpha:
  desc: hand listed
beta:
  path: ~/b
`,
			want: `alpha:
  desc: hand listed
  repos:
    - dir: ~/new
      src: git@h:o/new.git
beta:
  path: ~/b
`,
		},
		{
			name:    "file without a trailing newline gets one",
			project: "alpha",
			in:      "alpha:\n  repos:\n    - dir: ~/x\n      src: git@x:a/x.git",
			want: `alpha:
  repos:
    - dir: ~/x
      src: git@x:a/x.git
    - dir: ~/new
      src: git@h:o/new.git
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte(tt.in), 0o644); err != nil {
				t.Fatalf("write config: %v", err)
			}
			doc, err := Load(path)
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			project, err := doc.FindProject(tt.project)
			if err != nil {
				t.Fatalf("findProject: %v", err)
			}
			if err := doc.AddRepo(project, "~/new", "git@h:o/new.git"); err != nil {
				t.Fatalf("addRepo: %v", err)
			}
			if err := doc.Save(path); err != nil {
				t.Fatalf("save: %v", err)
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read back: %v", err)
			}
			if string(got) != tt.want {
				t.Errorf("config after add:\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}

// TestAddProjectPreservesFile proves that a new project is appended after
// everything the file already holds, and that a file with nothing but
// comments keeps them above the project that starts it.
func TestAddProjectPreservesFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "appended after existing projects and their comments",
			in: `# top
alpha:
  path: ~/a

# the end
`,
			want: `# top
alpha:
  path: ~/a

# the end
fresh:
  repos:
    - dir: ~/new
      src: git@h:o/new.git
`,
		},
		{
			name: "empty file",
			in:   "",
			want: `fresh:
  repos:
    - dir: ~/new
      src: git@h:o/new.git
`,
		},
		{
			name: "absent file",
			in:   "-",
			want: `fresh:
  repos:
    - dir: ~/new
      src: git@h:o/new.git
`,
		},
		{
			name: "comment only file keeps its comment",
			in:   "# my projects\n",
			want: `# my projects
fresh:
  repos:
    - dir: ~/new
      src: git@h:o/new.git
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "config.yaml")
			if tt.in != "-" {
				if err := os.WriteFile(path, []byte(tt.in), 0o644); err != nil {
					t.Fatalf("write config: %v", err)
				}
			}
			doc, err := Load(path)
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			project, err := doc.AddProject("fresh")
			if err != nil {
				t.Fatalf("addProject: %v", err)
			}
			if err := doc.AddRepo(project, "~/new", "git@h:o/new.git"); err != nil {
				t.Fatalf("addRepo: %v", err)
			}
			if err := doc.Save(path); err != nil {
				t.Fatalf("save: %v", err)
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read back: %v", err)
			}
			if string(got) != tt.want {
				t.Errorf("config after add:\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}

// TestLoadRejectsNonMapping covers a config file whose top level is not a
// mapping of projects, which nothing could be appended to.
func TestLoadRejectsNonMapping(t *testing.T) {
	t.Parallel()

	for _, in := range []string{"- a\n- b\n", "just a string\n", "a: 1\n---\nb: 2\n"} {
		path := filepath.Join(t.TempDir(), "config.yaml")
		if err := os.WriteFile(path, []byte(in), 0o644); err != nil {
			t.Fatalf("write config: %v", err)
		}
		if _, err := Load(path); err == nil {
			t.Errorf("load(%q) error = nil, want a rejected top level", in)
		}
	}
}

// TestSaveKeepsExamplesByteForByte proves that the example configs, which use
// every documented setting with comments and blank lines throughout, survive
// a load and save with nothing changed. This is what lets `gits add` promise
// a diff of nothing but the lines it appends.
func TestSaveKeepsExamplesByteForByte(t *testing.T) {
	t.Parallel()

	examples, err := filepath.Glob(filepath.Join("..", "..", "..", "examples", "*.yaml"))
	if err != nil || len(examples) == 0 {
		t.Fatalf("no example configs found: %v", err)
	}
	for _, example := range examples {
		t.Run(filepath.Base(example), func(t *testing.T) {
			t.Parallel()
			want, err := os.ReadFile(example)
			if err != nil {
				t.Fatalf("read example: %v", err)
			}
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, want, 0o644); err != nil {
				t.Fatalf("write config: %v", err)
			}
			doc, err := Load(path)
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			if err := doc.Save(path); err != nil {
				t.Fatalf("save: %v", err)
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read back: %v", err)
			}
			if string(got) != string(want) {
				t.Errorf("save changed %s without an edit:\n%s", example, got)
			}
		})
	}
}

// TestProjectNamesThatAreNotStrings covers a project whose name YAML would
// read as something other than a string — a directory called `123`, `yes` or
// `null`, which `gits discover` names a project after.
//
// Such a key is written quoted, and the syntax tree gives its source form
// back with the quotes still on. Looking it up by the name the caller asked
// for therefore has to compare what the key *means*, not how it is spelled:
// before it did, AddProjectPath wrote the key and then failed to find the
// project it had just written.
func TestProjectNamesThatAreNotStrings(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"123", "yes", "no", "null", "on", "off", "true", "1.5", "my project", "dot.name"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
				t.Fatalf("write config: %v", err)
			}
			doc, err := Load(path)
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			if doc.HasProject(name) {
				t.Errorf("HasProject(%q) = true on an empty config, want false", name)
			}
			if err := doc.AddProjectRepos(
				name, "~/code/"+name, []Repo{{Dir: "one", Src: "git@h:o/one.git"}},
			); err != nil {
				t.Fatalf("AddProjectRepos(%q): %v", name, err)
			}
			// The name must be found again by the name it was added under,
			// which is what a second run's duplicate check relies on.
			if !doc.HasProject(name) {
				t.Errorf("HasProject(%q) = false after adding it, want true", name)
			}
			if _, err := doc.FindProject(name); err != nil {
				t.Errorf("FindProject(%q) after adding it: %v", name, err)
			}
			if err := doc.Save(path); err != nil {
				t.Fatalf("save: %v", err)
			}

			// And the file must still say so after a round trip.
			reloaded, err := Load(path)
			if err != nil {
				t.Fatalf("reload: %v", err)
			}
			if !reloaded.HasProject(name) {
				data, _ := os.ReadFile(path)
				t.Errorf("HasProject(%q) = false after a round trip, want true:\n%s", name, data)
			}
		})
	}
}

// TestAddRepoToAQuotedProjectName proves `gits add` reaches a project whose
// name is quoted in the file, so the two commands agree about what a project
// is called.
func TestAddRepoToAQuotedProjectName(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("\"123\":\n  repos:\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	doc, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	project, err := doc.FindProject("123")
	if err != nil {
		t.Fatalf("FindProject(123): %v", err)
	}
	if err := doc.AddRepo(project, "~/code/x", "git@h:o/x.git"); err != nil {
		t.Fatalf("AddRepo: %v", err)
	}
	if err := doc.Save(path); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	want := "\"123\":\n  repos:\n    - dir: ~/code/x\n      src: git@h:o/x.git\n"
	if string(got) != want {
		t.Errorf("config = %q, want %q", got, want)
	}
}

// TestAddProjectRepos checks that a project is written with a path and its
// repositories listed.
func TestAddProjectRepos(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("# my config\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	doc, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if err := doc.AddProjectRepos("src", "~/src", []Repo{
		{Dir: "repo3", Src: "git@h:o/repo3.git"},
		// No remote: written without `src:`.
		{Dir: "repo6"},
	}); err != nil {
		t.Fatalf("AddProjectRepos: %v", err)
	}
	if err := doc.AddProjectRepos("project1", "~/src/project1", []Repo{
		{Dir: "repo1", Src: "https://h/o/repo1.git"},
	}); err != nil {
		t.Fatalf("AddProjectRepos: %v", err)
	}
	if err := doc.Save(path); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	want := `# my config
src:
  path: ~/src
  repos:
    - dir: repo3
      src: git@h:o/repo3.git
    - dir: repo6
project1:
  path: ~/src/project1
  repos:
    - dir: repo1
      src: https://h/o/repo1.git
`
	if string(got) != want {
		t.Errorf("config =\n%s\nwant:\n%s", got, want)
	}
}
