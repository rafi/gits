package providers

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/rafi/gits/domain"
)

// TestGitLabLoadReposCancelledCtx proves T7 (#2): GitLab requests carry the
// caller's context via gitlab.WithContext, so a pre-cancelled context aborts
// the very first call (GetGroup) promptly with context.Canceled instead of
// hitting the network.
func TestGitLabLoadReposCancelledCtx(t *testing.T) {
	p, err := newGitLabProvider(Options{Token: "dummy-token"})
	if err != nil {
		t.Fatalf("newGitLabProvider: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before the call, so no request must run

	done := make(chan error, 1)
	go func() {
		done <- p.LoadRepos(ctx, "somegroup", nil, &domain.Project{})
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("LoadRepos with a cancelled ctx = nil error, want context.Canceled")
		}
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("LoadRepos error = %v, want it to wrap context.Canceled", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("LoadRepos did not abort promptly on a cancelled context")
	}
}
