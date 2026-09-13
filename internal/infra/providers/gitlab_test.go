package providers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
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

// newTestGitLabProvider builds a provider pointed at a local test server.
func newTestGitLabProvider(t *testing.T, url string) *gitLabProvider {
	t.Helper()
	client, err := gitlab.NewClient("dummy", gitlab.WithBaseURL(url))
	if err != nil {
		t.Fatalf("gitlab.NewClient: %v", err)
	}
	return &gitLabProvider{client: client}
}

// TestGitLabLoadReposFlatWalk proves 13: a two-level group tree costs one
// GetGroup, one descendant-groups walk and one include_subgroups project
// walk — not a request pair per group — and still yields the same tree.
func TestGitLabLoadReposFlatWalk(t *testing.T) {
	t.Parallel()

	var paths []string
	var includeSubGroups string
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			paths = append(paths, r.URL.Path)
			w.Header().Set("Content-Type", "application/json")
			switch {
			case strings.HasSuffix(r.URL.Path, "/groups/acme"):
				fmt.Fprint(w, `{"id":1,"name":"Acme","path":"acme","full_path":"acme"}`)
			case strings.HasSuffix(r.URL.Path, "/descendant_groups"):
				fmt.Fprint(w, `[
					{"id":2,"name":"Backend","path":"backend","full_path":"acme/backend"},
					{"id":4,"name":"Deep","path":"deep","full_path":"acme/backend/deep"},
					{"id":3,"name":"Frontend","path":"frontend","full_path":"acme/frontend"}
				]`)
			case strings.HasSuffix(r.URL.Path, "/projects"):
				includeSubGroups = r.URL.Query().Get("include_subgroups")
				fmt.Fprint(w, `[
					{"id":10,"path":"top","namespace":{"full_path":"acme"},"ssh_url_to_repo":"git@x:acme/top.git"},
					{"id":11,"path":"api","namespace":{"full_path":"acme/backend"},"ssh_url_to_repo":"git@x:acme/backend/api.git"},
					{"id":12,"path":"core","namespace":{"full_path":"acme/backend/deep"},"ssh_url_to_repo":"git@x:acme/backend/deep/core.git"},
					{"id":13,"path":"web","namespace":{"full_path":"acme/frontend"},"ssh_url_to_repo":"git@x:acme/frontend/web.git"}
				]`)
			default:
				t.Errorf("unexpected request path %q", r.URL.Path)
			}
		}))
	defer server.Close()

	p := newTestGitLabProvider(t, server.URL)
	project := &domain.Project{}
	if err := p.LoadRepos(context.Background(), "acme", project); err != nil {
		t.Fatalf("LoadRepos: %v", err)
	}

	if len(paths) != 3 {
		t.Errorf("LoadRepos made %d requests (%v), want 3", len(paths), paths)
	}
	if includeSubGroups != "true" {
		t.Errorf("include_subgroups = %q, want \"true\"", includeSubGroups)
	}
	if project.Name != "Acme" {
		t.Errorf("project.Name = %q, want \"Acme\"", project.Name)
	}

	got := flattenGitLabTree(project, "")
	want := []string{
		"/top(10)",
		"backend(2)/api(11)",
		"backend/deep(4)/core(12)",
		"frontend(3)/web(13)",
	}
	if len(got) != len(want) {
		t.Fatalf("tree = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("tree[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// flattenGitLabTree renders "<group path>(<group id>)/<repo>(<repo id>)" for
// every repository, depth-first, so tree shape and identity compare as text.
func flattenGitLabTree(p *domain.Project, prefix string) []string {
	lines := []string{}
	for _, repo := range p.Repos {
		lines = append(lines, fmt.Sprintf("%s/%s(%s)", prefix, repo.Name, repo.ID))
	}
	for i := range p.SubProjects {
		sub := &p.SubProjects[i]
		subPrefix := sub.Name + "(" + sub.ID + ")"
		if prefix != "" {
			subPrefix = strings.SplitN(prefix, "(", 2)[0] + "/" + subPrefix
		}
		lines = append(lines, flattenGitLabTree(sub, subPrefix)...)
	}
	return lines
}
