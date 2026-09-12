package providers

import (
	"testing"

	"github.com/rafi/gits/domain"
)

func TestNewFilesystemRepo(t *testing.T) {
	t.Parallel()

	t.Run("name from path, src passthrough", func(t *testing.T) {
		t.Parallel()

		repo, err := NewFilesystemRepo("/code/myrepo", "git@host:o/r.git")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if repo.Name != "myrepo" {
			t.Errorf("Name = %q, want %q", repo.Name, "myrepo")
		}
		if repo.Dir != "/code/myrepo" {
			t.Errorf("Dir = %q, want %q", repo.Dir, "/code/myrepo")
		}
		if repo.Src != "git@host:o/r.git" {
			t.Errorf("Src = %q, want passthrough remote", repo.Src)
		}
		if repo.State == domain.RepoStateError {
			t.Errorf("State = %q, want non-error", repo.State)
		}
	})

	// An empty Repo Src is left empty rather than resolved from the clone's
	// own remote: reading it costs a git subprocess, deferred to the commands
	// that display Src (see loader.ResolveSrc). No git client is consulted here.
	t.Run("empty src stays empty, no git subprocess", func(t *testing.T) {
		t.Parallel()

		repo, err := NewFilesystemRepo("/code/myrepo", "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if repo.Src != "" {
			t.Errorf("Src = %q, want empty (resolution is lazy)", repo.Src)
		}
		if repo.State == domain.RepoStateError {
			t.Errorf("State = %q, want non-error", repo.State)
		}
	})

	// A user-specific home dir cannot be expanded and is the one input that
	// makes NewFilesystemRepo return an error.
	t.Run("unexpandable path errors", func(t *testing.T) {
		t.Parallel()

		if _, err := NewFilesystemRepo("~nobody-else/repo", ""); err == nil {
			t.Error("NewFilesystemRepo(unexpandable path) = nil, want error")
		}
	})
}
