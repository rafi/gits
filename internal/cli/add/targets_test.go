package add

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestExpandRepositoryTargetsGlob(t *testing.T) {
	t.Parallel()

	cwd := t.TempDir()
	for _, name := range []string{"backend-api", "backend-docs", "backend-worker", "frontend"} {
		if err := os.Mkdir(filepath.Join(cwd, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	isRepo := func(path string) bool {
		return filepath.Base(path) != "backend-docs"
	}
	got, err := expandRepositoryTargets(cwd, []string{"backend*"}, isRepo)
	if err != nil {
		t.Fatalf("expandRepositoryTargets() error = %v", err)
	}

	want := []string{
		filepath.Join(cwd, "backend-api"),
		filepath.Join(cwd, "backend-worker"),
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("expandRepositoryTargets() = %v, want %v", got, want)
	}
}

func TestExpandRepositoryTargetsExpandedByShell(t *testing.T) {
	t.Parallel()

	cwd := t.TempDir()
	for _, name := range []string{"backend-api", "backend-worker"} {
		if err := os.Mkdir(filepath.Join(cwd, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	got, err := expandRepositoryTargets(
		cwd,
		[]string{"backend-api", "backend-worker", "backend-api"},
		func(string) bool { return true },
	)
	if err != nil {
		t.Fatalf("expandRepositoryTargets() error = %v", err)
	}

	want := []string{
		filepath.Join(cwd, "backend-api"),
		filepath.Join(cwd, "backend-worker"),
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("expandRepositoryTargets() = %v, want %v", got, want)
	}
}

func TestExpandRepositoryTargetsNoMatches(t *testing.T) {
	t.Parallel()

	_, err := expandRepositoryTargets(
		t.TempDir(),
		[]string{"backend*"},
		func(string) bool { return true },
	)
	if err == nil {
		t.Fatal("expandRepositoryTargets() error = nil, want an error")
	}
}
