package add

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestLoadInitializesDefaultConfig(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	node, configPath, created, err := load("", homeDir)
	if err != nil {
		t.Fatalf("load() error = %v", err)
	}
	if !created {
		t.Error("load() created = false, want true")
	}

	wantPath := filepath.Join(homeDir, ".gits.yaml")
	if configPath != wantPath {
		t.Errorf("load() configPath = %q, want %q", configPath, wantPath)
	}
	if len(node.Content) == 0 || node.Content[0].Kind != yaml.MappingNode {
		t.Fatalf("load() did not return a YAML mapping: %#v", node)
	}
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Errorf("load() wrote the config before save, stat error = %v", err)
	}
}

func TestLoadAcceptsEmptyConfig(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	node, gotPath, created, err := load(configPath, "")
	if err != nil {
		t.Fatalf("load() error = %v", err)
	}
	if created {
		t.Error("load() created = true, want false")
	}
	if gotPath != configPath {
		t.Errorf("load() configPath = %q, want %q", gotPath, configPath)
	}
	if len(node.Content) == 0 || node.Content[0].Kind != yaml.MappingNode {
		t.Fatalf("load() did not return a YAML mapping: %#v", node)
	}
}

func TestSaveCreatesConfigDirectory(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	node, _, _, err := load("", homeDir)
	if err != nil {
		t.Fatalf("load() error = %v", err)
	}
	appendProject("backend", node.Content[0])

	configPath := filepath.Join(homeDir, ".config", "gits", "config.yaml")
	if err := save(configPath, node); err != nil {
		t.Fatalf("save() error = %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "backend:\n  repos: []\n" {
		t.Errorf("save() content = %q", data)
	}
}
