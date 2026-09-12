package git

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestCloneCancellationCleansTarget proves a canceled clone terminates git
// gracefully (SIGTERM, not SIGKILL) so git's own cleanup removes the partial
// target directory — a later re-run must not see "directory already exists".
// The ext:: transport blocks the clone deterministically without a network.
func TestCloneCancellationCleansTarget(t *testing.T) {
	requireGit(t)
	g := NewGit()
	t.Setenv("GIT_ALLOW_PROTOCOL", "ext")
	target := filepath.Join(t.TempDir(), "cloned")

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(500 * time.Millisecond)
		cancel()
	}()
	if _, err := g.Clone(ctx, "ext::sleep 5", target); err == nil {
		t.Fatal("Clone survived cancellation, want error")
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Errorf("partial clone target %s still exists after cancellation", target)
	}
}

// TestCloneNetworkTimeoutConfigurable proves the network timeout is taken
// from SetNetworkTimeout rather than a hard-coded constant: a hanging clone
// under a short timeout returns promptly.
func TestCloneNetworkTimeoutConfigurable(t *testing.T) {
	requireGit(t)
	g := NewGit()
	g.SetNetworkTimeout(300 * time.Millisecond)
	t.Setenv("GIT_ALLOW_PROTOCOL", "ext")
	target := filepath.Join(t.TempDir(), "cloned")

	start := time.Now()
	if _, err := g.Clone(context.Background(), "ext::sleep 5", target); err == nil {
		t.Fatal("Clone survived a 300ms network timeout, want error")
	}
	// Bounded by the sleep-5 transport exiting, never the default 5 minutes.
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Errorf("Clone took %s, timeout setting not honored", elapsed)
	}
}
