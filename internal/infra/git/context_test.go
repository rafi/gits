package git

import (
	"context"
	"testing"
	"time"
)

// TestContextCancellationAbortsOperation proves that git operations derive
// their execution context from the caller-supplied context: an already
// canceled context must abort the operation promptly rather than running to
// completion or the per-op timeout.
func TestContextCancellationAbortsOperation(t *testing.T) {
	t.Parallel()

	requireGit(t)
	g := NewGit()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before invoking, so the command must not run

	start := time.Now()
	if _, err := g.HeadInfo(ctx, "."); err == nil {
		t.Fatal("expected error from a canceled context, got nil")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("canceled operation did not abort promptly: took %s", elapsed)
	}
}
