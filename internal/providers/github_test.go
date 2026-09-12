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
	"time"

	"github.com/shurcooL/githubv4"
)

// newTestGitHubProvider builds a provider whose GraphQL client talks to a
// local httptest server instead of api.github.com.
func newTestGitHubProvider(url string, includeArchived bool) *gitHubProvider {
	return &gitHubProvider{
		client:          githubv4.NewEnterpriseClient(url, http.DefaultClient),
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
	t.Parallel()

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
	t.Parallel()

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
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
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

// TestRateLimitDelay covers the pacing rule in isolation: it is a pure
// function of the budget GitHub reported and the clock.
func TestRateLimitDelay(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	at := func(d time.Duration) githubv4.DateTime {
		return githubv4.DateTime{Time: now.Add(d)}
	}

	tests := []struct {
		name string
		rl   githubRateLimit
		want time.Duration
	}{
		// The normal case, and the point of the whole change: a healthy
		// budget pays nothing per page.
		{
			"pages to spare do not wait",
			githubRateLimit{Cost: 1, Remaining: 5000, ResetAt: at(time.Hour)},
			0,
		},
		{
			"a budget exactly at the floor does not wait",
			githubRateLimit{Cost: 1, Remaining: githubPageFloor, ResetAt: at(time.Hour)},
			0,
		},
		// 50 points at 1 each is 50 more pages; 10 minutes spread over 50
		// pages is 12s apiece.
		{
			"under the floor, paces the window across affordable pages",
			githubRateLimit{Cost: 1, Remaining: 50, ResetAt: at(10 * time.Minute)},
			12 * time.Second,
		},
		// The floor counts pages, not points: the same 500 points buy 500
		// pages of a cost-1 query and only 50 of a cost-10 one, and only the
		// latter is close enough to the edge to pace.
		{
			"a costlier page reaches the floor sooner",
			githubRateLimit{Cost: 10, Remaining: 500, ResetAt: at(10 * time.Minute)},
			12 * time.Second,
		},
		{
			"a cheap page on the same points does not wait",
			githubRateLimit{Cost: 1, Remaining: 500, ResetAt: at(10 * time.Minute)},
			0,
		},
		{
			"nothing affordable waits out the window, padded",
			githubRateLimit{Cost: 5, Remaining: 2, ResetAt: at(time.Minute)},
			time.Minute + githubResetPad,
		},
		{
			"an empty budget waits out the window, padded",
			githubRateLimit{Cost: 1, Remaining: 0, ResetAt: at(time.Minute)},
			time.Minute + githubResetPad,
		},
		// The window has closed: the budget is restored, or is about to be.
		{
			"a past resetAt does not wait",
			githubRateLimit{Cost: 1, Remaining: 0, ResetAt: at(-time.Minute)},
			0,
		},
		// A response with no rateLimit block at all decodes to this, and it
		// must not strand the walk on a zero-time window.
		{
			"an absent rateLimit block does not wait",
			githubRateLimit{},
			0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := rateLimitDelay(tt.rl, now); got != tt.want {
				t.Errorf("rateLimitDelay() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestGitHubFetchReposAsksForBudget proves the query carries the rateLimit
// block — without it there is nothing to pace from.
func TestGitHubFetchReposAsksForBudget(t *testing.T) {
	t.Parallel()

	var queries []string
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			var req ghRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Errorf("decode request: %v", err)
			}
			queries = append(queries, req.Query)
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w,
				`{"data":{"rateLimit":{"cost":1,"remaining":4999,"resetAt":"2026-08-23T13:00:00Z"},"repositoryOwner":{"id":"U_1","login":"acme","repositories":{"nodes":[%s],"pageInfo":{"endCursor":"","hasNextPage":false}}}}}`,
				ghRepoNodes(1, 1))
		},
	))
	defer server.Close()

	if _, _, err := newTestGitHubProvider(server.URL, false).fetchRepos(context.Background(), "acme"); err != nil {
		t.Fatalf("fetchRepos() error = %v", err)
	}
	if len(queries) != 1 {
		t.Fatalf("queries sent = %d, want 1", len(queries))
	}
	for _, field := range []string{"rateLimit", "cost", "remaining", "resetAt"} {
		if !strings.Contains(queries[0], field) {
			t.Errorf("query does not ask for %q: %s", field, queries[0])
		}
	}
}

// TestGitHubFetchReposPacesOnLowBudget proves the reported budget reaches the
// pause: with one page left to afford, the second page is spread across the
// whole of what remains of the window. The clock is fixed so the wait is
// exact.
func TestGitHubFetchReposPacesOnLowBudget(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	const window = 30 * time.Millisecond
	resetAt := now.Add(window).Format(time.RFC3339Nano)

	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			var req ghRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Errorf("decode request: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			hasNext := req.Variables.Cursor == nil
			fmt.Fprintf(w,
				`{"data":{"rateLimit":{"cost":1,"remaining":1,"resetAt":%q},"repositoryOwner":{"id":"U_1","login":"acme","repositories":{"nodes":[%s],"pageInfo":{"endCursor":"c1","hasNextPage":%t}}}}}`,
				resetAt, ghRepoNodes(1, 1), hasNext)
		},
	))
	defer server.Close()

	p := newTestGitHubProvider(server.URL, false)
	p.nowFunc = func() time.Time { return now }

	start := time.Now()
	repos, _, err := p.fetchRepos(context.Background(), "acme")
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("fetchRepos() error = %v", err)
	}
	if len(repos) != 2 {
		t.Fatalf("len(repos) = %d, want 2 (both pages consumed)", len(repos))
	}
	if elapsed < window {
		t.Errorf("fetchRepos took %v, want at least the %v window it was told to wait out", elapsed, window)
	}
}

// TestGitHubFetchReposHealthyBudgetIsNotTaxed proves the fixed per-page sleep
// is gone: three pages on a full budget cost only their round-trips. The old
// 100ms-per-page tax would have spent 200ms on the two gaps alone.
func TestGitHubFetchReposHealthyBudgetIsNotTaxed(t *testing.T) {
	t.Parallel()

	page := 0
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			page++
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w,
				`{"data":{"rateLimit":{"cost":1,"remaining":4999,"resetAt":"2026-08-23T13:00:00Z"},"repositoryOwner":{"id":"U_1","login":"acme","repositories":{"nodes":[%s],"pageInfo":{"endCursor":"c1","hasNextPage":%t}}}}}`,
				ghRepoNodes(page, page), page < 3)
		},
	))
	defer server.Close()

	p := newTestGitHubProvider(server.URL, false)
	p.nowFunc = func() time.Time { return time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC) }

	start := time.Now()
	repos, _, err := p.fetchRepos(context.Background(), "acme")
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("fetchRepos() error = %v", err)
	}
	if len(repos) != 3 {
		t.Fatalf("len(repos) = %d, want 3", len(repos))
	}
	// Generous: three local round-trips are sub-millisecond, and the tax
	// this replaces would have been 200ms.
	if elapsed > 100*time.Millisecond {
		t.Errorf("fetchRepos took %v over 3 pages, want no per-page delay", elapsed)
	}
}

// TestGitHubFetchReposCancelledCtx proves T6 (#2): the GitHub fetch derives its
// request from the caller's context. A pre-canceled context must abort before
// any network round-trip and surface [context.Canceled].
func TestGitHubFetchReposCancelledCtx(t *testing.T) {
	t.Parallel()

	p := newGitHubProvider(Options{Token: "dummy-token"})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before the call, so the query must not run

	_, _, err := p.fetchRepos(ctx, "anyorg")
	if err == nil {
		t.Fatal("fetchRepos with a canceled ctx = nil error, want context.Canceled")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("fetchRepos error = %v, want it to wrap context.Canceled", err)
	}
}
