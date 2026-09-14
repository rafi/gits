package add

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

	config, err := load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if err := config.save(path); err != nil {
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
			config, err := load(path)
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			project, err := config.findProject(tt.project)
			if err != nil {
				t.Fatalf("findProject: %v", err)
			}
			if err := config.addRepo(project, "~/new", "git@h:o/new.git"); err != nil {
				t.Fatalf("addRepo: %v", err)
			}
			if err := config.save(path); err != nil {
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
			config, err := load(path)
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			project, err := config.addProject("fresh")
			if err != nil {
				t.Fatalf("addProject: %v", err)
			}
			if err := config.addRepo(project, "~/new", "git@h:o/new.git"); err != nil {
				t.Fatalf("addRepo: %v", err)
			}
			if err := config.save(path); err != nil {
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
		if _, err := load(path); err == nil {
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

	examples, err := filepath.Glob(filepath.Join("..", "..", "..", "..", "..", "examples", "*.yaml"))
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
			config, err := load(path)
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			if err := config.save(path); err != nil {
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
