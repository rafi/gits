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
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := "myproj:\n  repos:\n    - dir: ~/code/x\n      src: git@x:a/x.git\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatalf("chmod: %v", err)
	}

	node, err := load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if err := save(path, node); err != nil {
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
