package git

import (
	"context"
	"testing"
)

// TestCheckoutRejectsFlagLikeRef proves refs are separated from flags: a ref
// beginning with "-" must reach git as a ref (and be rejected as unknown),
// never be parsed as a git flag. Without `--end-of-options`, `git checkout -f`
// silently force-checks-out the current branch and returns nil.
func TestCheckoutRejectsFlagLikeRef(t *testing.T) {
	g := NewGit()
	ctx := context.Background()
	dir := setupRepo(t)
	if err := g.Checkout(ctx, dir, "-f"); err == nil {
		t.Fatal(`Checkout with flag-like ref "-f" = nil, want error (ref must not be parsed as a flag)`)
	}
}

// TestDiffRejectsFlagLikeRef proves the rev-list range token cannot smuggle a
// git flag: a "-"-prefixed branch must be treated as a revision (unknown),
// never as an option.
func TestDiffRejectsFlagLikeRef(t *testing.T) {
	g := NewGit()
	ctx := context.Background()
	dir := setupRepo(t)
	if _, _, err := g.Diff(ctx, dir, "--all", "main"); err == nil {
		t.Fatal(`Diff with flag-like branch "--all" = nil, want error`)
	}
}
