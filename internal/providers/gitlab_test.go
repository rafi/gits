package providers

import (
	"context"
	"errors"
	"testing"
	"time"

	gitlab "gitlab.com/gitlab-org/api/client-go"

	"github.com/rafi/gits/domain"
)

// TestGitLabLoadReposCancelledCtx proves T7 (#2): GitLab requests carry the
// caller's context via gitlab.WithContext, so a pre-canceled context aborts
// the very first call (GetGroup) promptly with [context.Canceled] instead of
// hitting the network.
func TestGitLabLoadReposCancelledCtx(t *testing.T) {
	t.Parallel()

	p, err := newGitLabProvider(Options{Token: "dummy-token"})
	if err != nil {
		t.Fatalf("newGitLabProvider: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before the call, so no request must run

	done := make(chan error, 1)
	go func() {
		done <- p.LoadRepos(ctx, "somegroup", &domain.Project{})
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("LoadRepos with a canceled ctx = nil error, want context.Canceled")
		}
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("LoadRepos error = %v, want it to wrap context.Canceled", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("LoadRepos did not abort promptly on a canceled context")
	}
}

// TestSkipGitLabProject proves settings.includeArchived is honored: archived
// projects are listed when it is set, while empty repositories are always
// skipped (nothing to clone).
func TestSkipGitLabProject(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		archived, empty bool
		includeArchived bool
		want            bool
	}{
		{"normal project kept", false, false, false, false},
		{"archived skipped by default", true, false, false, true},
		{"archived kept when included", true, false, true, false},
		{"empty always skipped", false, true, true, true},
		{"archived and empty skipped", true, true, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := &gitlab.Project{Archived: tt.archived, EmptyRepo: tt.empty}
			if got := skipGitLabProject(p, tt.includeArchived); got != tt.want {
				t.Errorf("skipGitLabProject(archived=%v, empty=%v, include=%v) = %v, want %v",
					tt.archived, tt.empty, tt.includeArchived, got, tt.want)
			}
		})
	}
}
