package providers

import (
	"context"
	"errors"
	"testing"
)

// TestGitHubFetchReposCancelledCtx proves T6 (#2): the GitHub fetch derives its
// request from the caller's context. A pre-cancelled context must abort before
// any network round-trip and surface context.Canceled.
func TestGitHubFetchReposCancelledCtx(t *testing.T) {
	p, err := newGitHubProvider("dummy-token")
	if err != nil {
		t.Fatalf("newGitHubProvider: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before the call, so the query must not run

	_, _, err = p.fetchRepos(ctx, "anyorg")
	if err == nil {
		t.Fatal("fetchRepos with a cancelled ctx = nil error, want context.Canceled")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("fetchRepos error = %v, want it to wrap context.Canceled", err)
	}
}
