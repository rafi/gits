package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/mitchellh/go-homedir"
)

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return path
}

func TestLoadConfigFormats(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		file    string
		content string
	}{
		{
			name: "json",
			file: "c.json",
			content: `{
				"myproj": {"desc": "hello", "path": "~/code"},
				"settings": {"cacheTTL": "24h", "workerCount": 4}
			}`,
		},
		{
			name:    "yaml",
			file:    "c.yaml",
			content: "myproj:\n  desc: hello\n  path: ~/code\nsettings:\n  cacheTTL: 24h\n  workerCount: 4\n",
		},
		{
			name:    "toml",
			file:    "c.toml",
			content: "[myproj]\ndesc = \"hello\"\npath = \"~/code\"\n\n[settings]\ncacheTTL = \"24h\"\nworkerCount = 4\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := writeTemp(t, tt.file, tt.content)
			f := &File{}
			if err := f.loadConfig(path); err != nil {
				t.Fatalf("loadConfig: %v", err)
			}
			proj, ok := f.Projects["myproj"]
			if !ok {
				t.Fatalf("project myproj not parsed; got %v", f.Projects)
			}
			if proj.Desc != "hello" {
				t.Errorf("desc = %q, want %q", proj.Desc, "hello")
			}
			if _, leaked := f.Projects["settings"]; leaked {
				t.Error("settings key leaked into Projects")
			}
			if f.Settings.CacheTTL != "24h" {
				t.Errorf("cacheTTL = %q, want %q", f.Settings.CacheTTL, "24h")
			}
			if f.Settings.WorkerCount != 4 {
				t.Errorf("workerCount = %d, want 4", f.Settings.WorkerCount)
			}
		})
	}
}

func TestLoadConfigProviderSettings(t *testing.T) {
	t.Parallel()

	path := writeTemp(t, "c.yaml",
		"p:\n  desc: x\nsettings:\n  includeArchived: true\n  providerTimeout: 90s\n")
	f := &File{}
	if err := NewConfigFromFile(path, f); err != nil {
		t.Fatalf("NewConfigFromFile: %v", err)
	}
	if !f.Settings.IncludeArchived {
		t.Error("includeArchived = false, want true")
	}
	if got, err := f.Settings.ProviderTimeoutDuration(); err != nil || got != 90*time.Second {
		t.Errorf("providerTimeout = %v (err %v), want 90s", got, err)
	}
}

// TestLoadConfigProviderTokens proves per-provider credentials parse from
// the settings block, in both spellings of the token command key.
func TestLoadConfigProviderTokens(t *testing.T) {
	t.Parallel()

	path := writeTemp(t, "c.yaml", `
p:
  desc: x
settings:
  github:
    token-cmd: pass tokens/github
  gitlab:
    tokenCommand: pass tokens/gitlab
  bitbucket:
    token: user:app-password
`)
	f := &File{}
	if err := NewConfigFromFile(path, f); err != nil {
		t.Fatalf("NewConfigFromFile: %v", err)
	}
	tests := []struct {
		provider    string
		wantToken   string
		wantCommand string
	}{
		{"github", "", "pass tokens/github"},
		{"gitlab", "", "pass tokens/gitlab"},
		{"bitbucket", "user:app-password", ""},
	}
	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			t.Parallel()

			auth := f.Settings.ProviderAuth(tt.provider)
			if auth.Token != tt.wantToken {
				t.Errorf("token = %q, want %q", auth.Token, tt.wantToken)
			}
			if got := auth.Command(); got != tt.wantCommand {
				t.Errorf("Command() = %q, want %q", got, tt.wantCommand)
			}
		})
	}
}

func TestLoadConfigUnsupportedExt(t *testing.T) {
	t.Parallel()

	path := writeTemp(t, "c.ini", "nope")
	f := &File{}
	if err := f.loadConfig(path); err == nil {
		t.Error("loadConfig(.ini) = nil, want unsupported-format error")
	}
}

// TestLoadConfigSourceAndReposRejected proves a project that sets both a
// Provider Source and an explicit repos list is rejected by name — for a
// top-level project and for a nested sub-project — rather than silently
// discarding the repos (remote sources) or appending them (filesystem).
func TestLoadConfigSourceAndReposRejected(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
	}{
		{
			name: "top-level project",
			content: `
both:
  source:
    type: github
    search: acme
  repos:
    - name: pinned
      dir: ~/code/pinned
`,
		},
		{
			name: "nested sub-project",
			content: `
parent:
  path: ~/code
  subprojects:
    - name: both
      source:
        type: gitlab
        search: group
      repos:
        - name: pinned
          dir: ~/code/pinned
`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := writeTemp(t, "c.yaml", tt.content)
			err := (&File{}).loadConfig(path)
			if err == nil {
				t.Fatal("loadConfig(source+repos) = nil, want error")
			}
			for _, want := range []string{"both", "source:", "repos:"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q missing %q", err.Error(), want)
				}
			}
		})
	}
}

func TestLoadConfigWorkerCountDefault(t *testing.T) {
	t.Parallel()

	path := writeTemp(t, "c.yaml", "myproj:\n  desc: hi\n")
	f := &File{}
	if err := NewConfigFromFile(path, f); err != nil {
		t.Fatalf("NewConfigFromFile: %v", err)
	}
	want := max(runtime.NumCPU(), 2)
	if f.Settings.WorkerCount != want {
		t.Errorf("default workerCount = %d, want %d", f.Settings.WorkerCount, want)
	}
}

func TestConvertDeprecatedProjectsKey(t *testing.T) {
	t.Parallel()

	path := writeTemp(t, "c.yaml", "projects:\n  legacy:\n    desc: old\n")
	f := &File{}
	if err := f.loadConfig(path); err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	// Deprecated projects are still captured.
	if _, ok := f.Projects["legacy"]; !ok {
		t.Errorf("legacy project not captured; got %v", f.Projects)
	}
	// The deprecation is recorded as a Warning for the caller to render on
	// Diagnostic Output, rather than printed to os.Stderr from here.
	if len(f.Warnings) != 1 {
		t.Fatalf("Warnings = %v, want one deprecation notice", f.Warnings)
	}
	if !strings.Contains(f.Warnings[0], "projects:") ||
		!strings.Contains(f.Warnings[0], "deprecated") {
		t.Errorf("Warnings[0] = %q, want the deprecation notice naming the key", f.Warnings[0])
	}
	// And Convert surfaces the deprecation error.
	if err := f.Convert(); err == nil {
		t.Error("Convert() = nil, want deprecation error")
	}
}

func TestFindDefaultPath(t *testing.T) {
	disableHomedirCache(t)

	t.Run("xdg config dir", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		xdg := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", xdg)
		dir := filepath.Join(xdg, "gits")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		want := filepath.Join(dir, "config.yaml")
		if err := os.WriteFile(want, []byte("{}"), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
		f := &File{}
		got, err := f.findDefaultPath()
		if err != nil {
			t.Fatalf("findDefaultPath: %v", err)
		}
		if got != want {
			t.Errorf("findDefaultPath = %q, want %q", got, want)
		}
	})

	t.Run("home .gits takes precedence", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		xdg := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", xdg)
		// Both candidates exist; ~/.gits.json is earlier in search order.
		want := filepath.Join(home, ".gits.json")
		if err := os.WriteFile(want, []byte("{}"), 0o644); err != nil {
			t.Fatalf("write home: %v", err)
		}
		xdgDir := filepath.Join(xdg, "gits")
		if err := os.MkdirAll(xdgDir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(xdgDir, "config.yaml"), []byte("{}"), 0o644); err != nil {
			t.Fatalf("write xdg: %v", err)
		}
		f := &File{}
		got, err := f.findDefaultPath()
		if err != nil {
			t.Fatalf("findDefaultPath: %v", err)
		}
		if got != want {
			t.Errorf("findDefaultPath = %q, want %q (home should win)", got, want)
		}
	})

	t.Run("none found returns empty", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		t.Setenv("XDG_CONFIG_HOME", t.TempDir())
		f := &File{}
		got, err := f.findDefaultPath()
		if err != nil {
			t.Fatalf("findDefaultPath: %v", err)
		}
		if got != "" {
			t.Errorf("findDefaultPath = %q, want empty", got)
		}
	})
}

// TestNewConfigDefaultsWithoutFile proves runtime defaults do not depend on a
// config file: without one, bulk commands must still get a real worker count
// and the -C color toggle must still be honored.
//
//nolint:paralleltest // restoreEnv rewrites process environment; must stay serial.
func TestNewConfigDefaultsWithoutFile(t *testing.T) {
	disableHomedirCache(t)

	origProfile := lipgloss.Writer.Profile
	origForce, hadForce := os.LookupEnv("CLICOLOR_FORCE")
	t.Cleanup(func() {
		lipgloss.Writer.Profile = origProfile
		restoreEnv(t, "CLICOLOR_FORCE", origForce, hadForce)
	})

	t.Run("no config file found", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		t.Setenv("XDG_CONFIG_HOME", t.TempDir())
		os.Unsetenv("CLICOLOR_FORCE")

		f := &File{Color: ColorOptionAlways.String()}
		if err := NewConfigFromFile("", f); err != nil {
			t.Fatalf("NewConfigFromFile: %v", err)
		}
		want := max(runtime.NumCPU(), 2)
		if f.Settings.WorkerCount != want {
			t.Errorf("workerCount = %d, want %d", f.Settings.WorkerCount, want)
		}
		if os.Getenv("CLICOLOR_FORCE") != "1" {
			t.Errorf("CLICOLOR_FORCE = %q, want 1", os.Getenv("CLICOLOR_FORCE"))
		}
	})

	t.Run("broken config file still applies defaults", func(t *testing.T) {
		path := writeTemp(t, "c.yaml", ":\tnot yaml [")
		f := &File{}
		if err := NewConfigFromFile(path, f); err == nil {
			t.Fatal("NewConfigFromFile = nil, want parse error")
		}
		want := max(runtime.NumCPU(), 2)
		if f.Settings.WorkerCount != want {
			t.Errorf("workerCount = %d, want %d", f.Settings.WorkerCount, want)
		}
	})
}

//nolint:paralleltest // restoreEnv rewrites process environment; must stay serial.
func TestLoadConfigColorToggle(t *testing.T) {
	origProfile := lipgloss.Writer.Profile
	origNoColor, hadNoColor := os.LookupEnv("NO_COLOR")
	origForce, hadForce := os.LookupEnv("CLICOLOR_FORCE")
	t.Cleanup(func() {
		lipgloss.Writer.Profile = origProfile
		restoreEnv(t, "NO_COLOR", origNoColor, hadNoColor)
		restoreEnv(t, "CLICOLOR_FORCE", origForce, hadForce)
	})

	t.Run("never forces NoTTY", func(t *testing.T) {
		os.Unsetenv("NO_COLOR")
		path := writeTemp(t, "c.yaml", "p:\n  desc: x\n")
		f := &File{Color: ColorOptionNever.String()}
		if err := NewConfigFromFile(path, f); err != nil {
			t.Fatalf("NewConfigFromFile: %v", err)
		}
		if lipgloss.Writer.Profile != colorprofile.NoTTY {
			t.Errorf("profile = %v, want NoTTY", lipgloss.Writer.Profile)
		}
		if os.Getenv("NO_COLOR") != "1" {
			t.Errorf("NO_COLOR = %q, want 1", os.Getenv("NO_COLOR"))
		}
	})

	t.Run("always forces TrueColor", func(t *testing.T) {
		os.Unsetenv("CLICOLOR_FORCE")
		path := writeTemp(t, "c.yaml", "p:\n  desc: x\n")
		f := &File{Color: ColorOptionAlways.String()}
		if err := NewConfigFromFile(path, f); err != nil {
			t.Fatalf("NewConfigFromFile: %v", err)
		}
		if lipgloss.Writer.Profile != colorprofile.TrueColor {
			t.Errorf("profile = %v, want TrueColor", lipgloss.Writer.Profile)
		}
		if os.Getenv("CLICOLOR_FORCE") != "1" {
			t.Errorf("CLICOLOR_FORCE = %q, want 1", os.Getenv("CLICOLOR_FORCE"))
		}
	})
}

// TestNewConfigFromFileErrors pins the failure cases the startup path turns
// into an exit code: a named-but-absent file, a malformed file and an
// unsupported extension are all errors. The non-error case — no file named and
// none found — is covered by TestNewConfigDefaultsWithoutFile.
func TestNewConfigFromFileErrors(t *testing.T) {
	t.Parallel()

	t.Run("named file that does not exist errors", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "absent.yaml")
		if err := NewConfigFromFile(path, &File{}); err == nil {
			t.Error("NewConfigFromFile(absent) = nil, want a read error")
		}
	})

	t.Run("malformed file errors", func(t *testing.T) {
		t.Parallel()

		path := writeTemp(t, "c.yaml", ":\tnot yaml [")
		if err := NewConfigFromFile(path, &File{}); err == nil {
			t.Error("NewConfigFromFile(malformed) = nil, want a parse error")
		}
	})

	t.Run("unsupported extension errors", func(t *testing.T) {
		t.Parallel()

		path := writeTemp(t, "c.ini", "nope")
		if err := NewConfigFromFile(path, &File{}); err == nil {
			t.Error("NewConfigFromFile(.ini) = nil, want an unsupported-format error")
		}
	})
}

// disableHomedirCache stops go-homedir answering from its process-wide cache,
// so a test that sets $HOME per case gets the home it just set.
func disableHomedirCache(t *testing.T) {
	t.Helper()
	//nolint:reassign // go-homedir exposes its cache toggle as a package variable.
	homedir.DisableCache = true
	t.Cleanup(func() {
		//nolint:reassign // restore the package default this test changed.
		homedir.DisableCache = false
	})
}

func restoreEnv(t *testing.T, key, val string, had bool) {
	t.Helper()
	if had {
		//nolint:usetesting // t.Setenv registers its own cleanup and cannot run from inside one.
		os.Setenv(key, val)
	} else {
		os.Unsetenv(key)
	}
}

// TestLoadConfigUnknownKeysWarn covers the load-time half of `gits doctor`:
// koanf ignores a key that matches no struct field, so `pth:` instead of
// `path:` produced a project with no path and no message at all. Every
// unknown key is now a warning naming it, on every run.
func TestLoadConfigUnknownKeysWarn(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		want    []string
	}{
		{
			name:    "misspelled project key",
			content: "acme:\n  pth: ~/code\n",
			want:    []string{"acme.pth"},
		},
		{
			name: "misspelled repository key",
			content: `
acme:
  path: ~/code
  repos:
    - name: one
      srcc: git@example.com:acme/one.git
`,
			want: []string{"acme.repos[0].srcc"},
		},
		{
			name: "misspelled sub-project key",
			content: `
acme:
  path: ~/code
  subprojects:
    - name: sub
      pathh: ~/other
`,
			want: []string{"acme.subprojects[0].pathh"},
		},
		{
			name:    "misspelled settings key",
			content: "settings:\n  workerCont: 4\n",
			want:    []string{"settings.workerCont"},
		},
		{
			name:    "misspelled nested settings key",
			content: "settings:\n  finder:\n    binry: sk\n",
			want:    []string{"settings.finder.binry"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := writeTemp(t, "c.yaml", tt.content)
			f := &File{}
			if err := f.loadConfig(path); err != nil {
				t.Fatalf("loadConfig: %v", err)
			}
			joined := strings.Join(f.Warnings, "\n")
			for _, want := range tt.want {
				if !strings.Contains(joined, want) {
					t.Errorf("warnings %q missing the unknown key %q", joined, want)
				}
			}
			// An unknown key is a warning, never a failure: the rest of the
			// config still loads and every command still runs.
			if len(f.Warnings) == 0 {
				t.Error("warnings = none, want one naming the unknown key")
			}
		})
	}
}

// TestLoadConfigKnownKeysDoNotWarn is the other half, and the one that keeps
// the check honest: a checker that flags the project's own documented example
// config is worse than no checker. Every key of both shipped examples, and of
// a config exercising each documented shape, must load silently.
func TestLoadConfigKnownKeysDoNotWarn(t *testing.T) {
	t.Parallel()

	t.Run("every documented shape", func(t *testing.T) {
		t.Parallel()

		path := writeTemp(t, "c.yaml", `
settings:
  cache: true
  cacheTTL: 168h
  providerTimeout: 5m
  gitTimeout: 5m
  workerCount: 8
  includeArchived: false
  verbose: false
  finder:
    binary: sk
    args: ["--ansi"]
    extra: ["--height=80%"]
  github:
    tokenCommand: pass tokens/github
  gitlab:
    token-cmd: op read op://private/gitlab/token
  bitbucket:
    token: user:app-password
  icons:
    modified: "M"
    gone: "G"
  theme:
    repoTitle: { color: "4", bold: true }
    diff: { color: "140", align: right, width: 3, faint: true }

remote:
  desc: A provider-backed project
  path: ~/code/github
  clone: false
  source:
    type: github
    search: rafi
  include: [one]
  exclude: [two]

explicit:
  path: ~/code/myapp
  repos:
    - id: "1"
      name: api
      namespace: acme
      dir: api
      src: https://example.com/acme/api.git
      url: https://example.com/acme/api
      desc: The API
  subprojects:
    - name: tools
      path: ~/code/tools
      repos:
        - dir: cli
`)
		f := &File{}
		if err := f.loadConfig(path); err != nil {
			t.Fatalf("loadConfig: %v", err)
		}
		if len(f.Warnings) != 0 {
			t.Errorf("warnings = %q, want none for a config using only documented keys", f.Warnings)
		}
	})

	// The shipped examples are the strongest regression test available: they
	// are what a user copies, so a false positive on them is a false positive
	// for everyone.
	for _, name := range []string{"config.yaml", "simple.yaml"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join("..", "..", "..", "examples", name)
			if _, err := os.Stat(path); err != nil {
				t.Skipf("example not present: %v", err)
			}
			f := &File{}
			if err := f.loadConfig(path); err != nil {
				t.Fatalf("loadConfig(%s): %v", name, err)
			}
			if len(f.Warnings) != 0 {
				t.Errorf("examples/%s warnings = %q, want none", name, f.Warnings)
			}
		})
	}
}

// TestLoadConfigUnknownKeysAreCaseInsensitive documents a limit of the check
// rather than asserting a behavior worth having: koanf matches keys
// case-insensitively, so a miscapitalized key still binds and is not reported.
// Pinned so a future change to strict matching is a deliberate one.
func TestLoadConfigUnknownKeysAreCaseInsensitive(t *testing.T) {
	t.Parallel()

	path := writeTemp(t, "c.yaml", "settings:\n  cachettl: 1h\n")
	f := &File{}
	if err := f.loadConfig(path); err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if len(f.Warnings) != 0 {
		t.Errorf("warnings = %q, want none: a miscapitalized key still binds", f.Warnings)
	}
	if f.Settings.CacheTTL != "1h" {
		t.Errorf("cacheTTL = %q, want %q — the value must still have bound", f.Settings.CacheTTL, "1h")
	}
}
