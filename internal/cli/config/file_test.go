package config

import (
	"os"
	"path/filepath"
	"runtime"
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
	path := writeTemp(t, "c.yaml",
		"p:\n  desc: x\nsettings:\n  includeArchived: true\n  providerTimeout: 90s\n")
	f := &File{}
	if err := NewConfigFromFile(path, f); err != nil {
		t.Fatalf("NewConfigFromFile: %v", err)
	}
	if !f.Settings.IncludeArchived {
		t.Error("includeArchived = false, want true")
	}
	if got := f.Settings.ProviderTimeoutDuration(); got != 90*time.Second {
		t.Errorf("providerTimeout = %v, want 90s", got)
	}
}

// TestLoadConfigProviderTokens proves per-provider credentials parse from
// the settings block, in both spellings of the token command key.
func TestLoadConfigProviderTokens(t *testing.T) {
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
	path := writeTemp(t, "c.ini", "nope")
	f := &File{}
	if err := f.loadConfig(path); err == nil {
		t.Error("loadConfig(.ini) = nil, want unsupported-format error")
	}
}

func TestLoadConfigWorkerCountDefault(t *testing.T) {
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
	path := writeTemp(t, "c.yaml", "projects:\n  legacy:\n    desc: old\n")
	f := &File{}
	if err := f.loadConfig(path); err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	// Deprecated projects are still captured.
	if _, ok := f.Projects["legacy"]; !ok {
		t.Errorf("legacy project not captured; got %v", f.Projects)
	}
	// And Convert surfaces the deprecation error.
	if err := f.Convert(); err == nil {
		t.Error("Convert() = nil, want deprecation error")
	}
}

func TestFindDefaultPath(t *testing.T) {
	homedir.DisableCache = true
	t.Cleanup(func() { homedir.DisableCache = false })

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
func TestNewConfigDefaultsWithoutFile(t *testing.T) {
	homedir.DisableCache = true
	t.Cleanup(func() { homedir.DisableCache = false })

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

func restoreEnv(t *testing.T, key, val string, had bool) {
	t.Helper()
	if had {
		os.Setenv(key, val)
	} else {
		os.Unsetenv(key)
	}
}
