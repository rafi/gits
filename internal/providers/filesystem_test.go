package providers

import (
	"context"
	"errors"
	"testing"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/git"
)

// fakeRemoteGit stubs only Remote; NewFilesystemRepo touches nothing else.
type fakeRemoteGit struct {
	git.Client

	remote    string
	remoteErr error
}

func (f fakeRemoteGit) Remote(context.Context, string) (string, error) {
	return f.remote, f.remoteErr
}

func TestNewFilesystemRepo(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("name from path, src passthrough", func(t *testing.T) {
		t.Parallel()

		g := fakeRemoteGit{remote: "should-not-be-used"}
		repo, err := NewFilesystemRepo(ctx, "/code/myrepo", "git@host:o/r.git", g)
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

	t.Run("empty src falls back to git remote", func(t *testing.T) {
		t.Parallel()

		g := fakeRemoteGit{remote: "git@host:o/derived.git"}
		repo, err := NewFilesystemRepo(ctx, "/code/myrepo", "", g)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if repo.Src != "git@host:o/derived.git" {
			t.Errorf("Src = %q, want git-remote fallback", repo.Src)
		}
		if repo.State == domain.RepoStateError {
			t.Errorf("State = %q, want non-error", repo.State)
		}
	})

	t.Run("remote error marks state error with reason", func(t *testing.T) {
		t.Parallel()

		g := fakeRemoteGit{remoteErr: errors.New("not a git repo")}
		repo, err := NewFilesystemRepo(ctx, "/code/myrepo", "", g)
		if err != nil {
			t.Fatalf("NewFilesystemRepo should not propagate remote error, got: %v", err)
		}
		if repo.State != domain.RepoStateError {
			t.Errorf("State = %q, want %q", repo.State, domain.RepoStateError)
		}
		if repo.Reason != "not a git repo" {
			t.Errorf("Reason = %q, want %q", repo.Reason, "not a git repo")
		}
	})
}
