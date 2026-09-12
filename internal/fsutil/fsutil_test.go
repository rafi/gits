package fsutil

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestWriteFileAtomic(t *testing.T) {
	t.Parallel()

	t.Run("writes content and applies the mode", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "cache.json")
		want := []byte(`{"hello":"world"}`)
		if err := WriteFileAtomic(path, want, 0o640); err != nil {
			t.Fatalf("WriteFileAtomic: %v", err)
		}

		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read back: %v", err)
		}
		if string(got) != string(want) {
			t.Errorf("content = %q, want %q", got, want)
		}

		fi, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat: %v", err)
		}
		if got := fi.Mode().Perm(); got != 0o640 {
			t.Errorf("mode = %v, want %v", got, os.FileMode(0o640))
		}
	})

	t.Run("replaces an existing file", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "cache.json")
		if err := os.WriteFile(path, []byte("stale"), 0o600); err != nil {
			t.Fatalf("seed: %v", err)
		}
		if err := WriteFileAtomic(path, []byte("fresh"), 0o600); err != nil {
			t.Fatalf("WriteFileAtomic: %v", err)
		}

		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read back: %v", err)
		}
		if string(got) != "fresh" {
			t.Errorf("content = %q, want %q", got, "fresh")
		}
	})

	// The temp file is created beside the target, so a directory that does
	// not exist fails at CreateTemp rather than at the rename.
	t.Run("missing directory is an error", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "nope", "cache.json")
		if err := WriteFileAtomic(path, []byte("x"), 0o600); err == nil {
			t.Fatal("WriteFileAtomic = nil, want an error for a missing directory")
		}
	})

	// A failure must not leave the temp file behind. An unwritable directory
	// is the reachable failure: CreateTemp fails, so there is nothing to
	// clean up, and the directory stays empty.
	t.Run("unwritable directory leaves nothing behind", func(t *testing.T) {
		t.Parallel()

		if runtime.GOOS == "windows" {
			t.Skip("directory permissions do not gate writes the same way")
		}
		if os.Geteuid() == 0 {
			t.Skip("root ignores directory permissions")
		}

		dir := t.TempDir()
		if err := os.Chmod(dir, 0o500); err != nil {
			t.Fatalf("chmod: %v", err)
		}
		t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

		if err := WriteFileAtomic(filepath.Join(dir, "cache.json"), []byte("x"), 0o600); err == nil {
			t.Fatal("WriteFileAtomic = nil, want an error for an unwritable directory")
		}

		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("readdir: %v", err)
		}
		if len(entries) != 0 {
			t.Errorf("directory holds %d entries, want 0 — no temp file may survive a failure", len(entries))
		}
	})
}

func TestWriteFileAtomicPreserve(t *testing.T) {
	t.Parallel()

	t.Run("keeps the mode of an existing file", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "config.yaml")
		if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
			t.Fatalf("seed: %v", err)
		}
		if err := os.Chmod(path, 0o640); err != nil {
			t.Fatalf("chmod: %v", err)
		}

		// The fallback mode differs, so a preserved mode is unambiguous.
		if err := WriteFileAtomicPreserve(path, []byte("new"), 0o600); err != nil {
			t.Fatalf("WriteFileAtomicPreserve: %v", err)
		}

		fi, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat: %v", err)
		}
		if got := fi.Mode().Perm(); got != 0o640 {
			t.Errorf("mode = %v, want %v — an existing mode must be preserved", got, os.FileMode(0o640))
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read back: %v", err)
		}
		if string(got) != "new" {
			t.Errorf("content = %q, want %q", got, "new")
		}
	})

	t.Run("absent file takes the fallback mode", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "config.yaml")
		if err := WriteFileAtomicPreserve(path, []byte("new"), 0o604); err != nil {
			t.Fatalf("WriteFileAtomicPreserve: %v", err)
		}

		fi, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat: %v", err)
		}
		if got := fi.Mode().Perm(); got != 0o604 {
			t.Errorf("mode = %v, want %v — an absent file takes the fallback", got, os.FileMode(0o604))
		}
	})
}
