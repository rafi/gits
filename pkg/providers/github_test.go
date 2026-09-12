package providers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/shurcooL/githubv4"
)

// newTestGitHubProvider builds a provider whose GraphQL client talks to a
// local httptest server instead of api.github.com.
func newTestGitHubProvider(url string, includeArchived bool) *gitHubProvider {
	return &gitHubProvider{
		client:          githubv4.NewEnterpriseClient(url, http.DefaultClient),
		sourceType:      ProviderGitHub,
		includeArchived: includeArchived,
	}
}

// ghRequest is the shape of a githubv4 POST body the fixtures care about.
type ghRequest struct {
	Query     string `json:"query"`
	Variables struct {
		Owner      string  `json:"owner"`
		Cursor     *string `json:"cursor"`
		IsArchived *bool   `json:"isArchived"`
	} `json:"variables"`
}

func ghRepoNodes(from, to int) string {
	nodes := []string{}
	for i := from; i <= to; i++ {
		nodes = append(nodes, fmt.Sprintf(
			`{"id":"R_%d","name":"repo-%02d","description":"d","url":"https://x/repo-%02d","sshUrl":"git@x:acme/repo-%02d.git"}`,
			i, i, i, i,
		))
	}
	return strings.Join(nodes, ",")
}

// TestGitHubFetchReposPagination proves the repositoryOwner connection is
// walked past the first page and works for user accounts as well as
// organizations (the old search-based org: qualifier matched orgs only).
func TestGitHubFetchReposPagination(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			var req ghRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Errorf("decode request: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			if req.Variables.Cursor == nil {
				fmt.Fprintf(w,
					`{"data":{"repositoryOwner":{"id":"U_1","login":"acme","repositories":{"nodes":[%s],"pageInfo":{"endCursor":"c1","hasNextPage":true}}}}}`,
					ghRepoNodes(1, 10))
				return
			}
			fmt.Fprintf(w,
				`{"data":{"repositoryOwner":{"id":"U_1","login":"acme","repositories":{"nodes":[%s],"pageInfo":{"endCursor":"c2","hasNextPage":false}}}}}`,
				ghRepoNodes(11, 15))
		},
	))
	defer server.Close()

	p := newTestGitHubProvider(server.URL, false)
	repos, ownerID, err := p.fetchRepos(context.Background(), "acme")
	if err != nil {
		t.Fatalf("fetchRepos() error = %v", err)
	}
	if len(repos) != 15 {
		t.Fatalf("len(repos) = %d, want 15 (both pages consumed)", len(repos))
	}
	if repos[0].Name != "repo-01" || repos[14].Name != "repo-15" {
		t.Errorf("repos span = %q..%q, want repo-01..repo-15", repos[0].Name, repos[14].Name)
	}
	if repos[0].Namespace != "acme" {
		t.Errorf("namespace = %q, want %q", repos[0].Namespace, "acme")
	}
	if ownerID != "U_1" {
		t.Errorf("ownerID = %q, want U_1 (from repositoryOwner, not a repo edge)", ownerID)
	}
}

// TestGitHubFetchReposArchivedFilter proves archived filtering happens
// server-side: isArchived=false is sent by default, and the includeArchived
// setting lifts the filter by sending null.
func TestGitHubFetchReposArchivedFilter(t *testing.T) {
	var gotArchived []*bool
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			var req ghRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Errorf("decode request: %v", err)
			}
			gotArchived = append(gotArchived, req.Variables.IsArchived)
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w,
				`{"data":{"repositoryOwner":{"id":"U_1","login":"acme","repositories":{"nodes":[%s],"pageInfo":{"endCursor":"","hasNextPage":false}}}}}`,
				ghRepoNodes(1, 1))
		},
	))
	defer server.Close()

	if _, _, err := newTestGitHubProvider(server.URL, false).fetchRepos(context.Background(), "acme"); err != nil {
		t.Fatalf("fetchRepos(default) error = %v", err)
	}
	if len(gotArchived) != 1 || gotArchived[0] == nil || *gotArchived[0] {
		t.Errorf("default isArchived var = %v, want false (filter archived out)", gotArchived)
	}

	gotArchived = nil
	if _, _, err := newTestGitHubProvider(server.URL, true).fetchRepos(context.Background(), "acme"); err != nil {
		t.Fatalf("fetchRepos(includeArchived) error = %v", err)
	}
	if len(gotArchived) != 1 || gotArchived[0] != nil {
		t.Errorf("includeArchived isArchived var = %v, want null (no filter)", gotArchived)
	}
}

// TestGitHubFetchReposUnknownOwner proves a null repositoryOwner (unknown
// login) surfaces a clear error instead of an empty result.
func TestGitHubFetchReposUnknownOwner(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"data":{"repositoryOwner":null}}`)
		},
	))
	defer server.Close()

	_, _, err := newTestGitHubProvider(server.URL, false).fetchRepos(context.Background(), "no-such-owner")
	if err == nil {
		t.Fatal("fetchRepos(unknown owner) = nil error, want error")
	}
	if !strings.Contains(err.Error(), "no-such-owner") {
		t.Errorf("error %q does not name the unknown owner", err)
	}
}

// TestGitHubFetchReposCancelledCtx proves T6 (#2): the GitHub fetch derives its
// request from the caller's context. A pre-cancelled context must abort before
// any network round-trip and surface context.Canceled.
func TestGitHubFetchReposCancelledCtx(t *testing.T) {
	p, err := newGitHubProvider(Options{Token: "dummy-token"})
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
