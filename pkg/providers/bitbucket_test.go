package providers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	bitbucket "github.com/ktrysmt/go-bitbucket"

	"github.com/rafi/gits/domain"
)

// TestBitbucketProviderTimeout proves the providerTimeout setting reaches the
// underlying HTTP client, which go-bitbucket otherwise leaves unbounded.
func TestBitbucketProviderTimeout(t *testing.T) {
	provider, err := newBitbucketProvider(Options{Token: "user:pass", Timeout: 42 * time.Second})
	if err != nil {
		t.Fatalf("newBitbucketProvider() error = %v", err)
	}
	if got := provider.client.HttpClient.Timeout; got != 42*time.Second {
		t.Errorf("HttpClient.Timeout = %v, want 42s", got)
	}
}

// TestBitbucketLoadReposCancelledCtx proves a pre-cancelled context aborts
// before any network round-trip: go-bitbucket hardcodes context.Background
// internally, so the provider must check at its own boundary.
func TestBitbucketLoadReposCancelledCtx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"pagelen":10,"page":1,"values":[%s]}`,
				`{"uuid":"{1}","slug":"a","owner":{"uuid":"{o}"}}`)
		},
	))
	defer server.Close()

	provider, err := newBitbucketProvider(Options{Token: "user:pass"})
	if err != nil {
		t.Fatalf("newBitbucketProvider() error = %v", err)
	}
	baseURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("url.Parse: %v", err)
	}
	provider.client.SetApiBaseURL(*baseURL)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before the call, so no request may be made

	err = provider.LoadRepos(ctx, "acme", &domain.Project{})
	if err == nil {
		t.Fatal("LoadRepos with cancelled ctx = nil error, want context.Canceled")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("LoadRepos error = %v, want it to wrap context.Canceled", err)
	}
}

// TestFetchReposPagination proves accounts spanning multiple API pages are
// fully listed: the client must follow the "next" link instead of stopping
// after the first page.
func TestFetchReposPagination(t *testing.T) {
	pageValues := func(from, to int) string {
		items := ""
		for i := from; i <= to; i++ {
			if items != "" {
				items += ","
			}
			items += fmt.Sprintf(
				`{"uuid":"{%d}","slug":"repo-%02d","owner":{"uuid":"{owner}"}}`, i, i,
			)
		}
		return items
	}

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if r.URL.Query().Get("page") == "2" {
				fmt.Fprintf(w, `{"pagelen":10,"page":2,"values":[%s]}`, pageValues(11, 15))
				return
			}
			fmt.Fprintf(w,
				`{"pagelen":10,"page":1,"next":%q,"values":[%s]}`,
				server.URL+r.URL.Path+"?page=2", pageValues(1, 10),
			)
		},
	))
	defer server.Close()

	provider, err := newBitbucketProvider(Options{Token: "user:pass"})
	if err != nil {
		t.Fatalf("newBitbucketProvider() error = %v", err)
	}
	baseURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("url.Parse(%q) error = %v", server.URL, err)
	}
	provider.client.SetApiBaseURL(*baseURL)

	repos, ownerID, err := provider.fetchRepos(context.Background(), "acme")
	if err != nil {
		t.Fatalf("fetchRepos() error = %v", err)
	}
	if len(repos) != 15 {
		t.Fatalf("len(repos) = %d, want 15 (both pages consumed)", len(repos))
	}
	if repos[0].Name != "repo-01" || repos[14].Name != "repo-15" {
		t.Errorf("repos span = %q..%q, want repo-01..repo-15",
			repos[0].Name, repos[14].Name)
	}
	if ownerID != "{owner}" {
		t.Errorf("ownerID = %q, want %q", ownerID, "{owner}")
	}
}

// TestParseReposMalformedNoPanic proves T5 (#1): malformed Owner/Links maps in
// an untrusted Bitbucket response are tolerated — fields are skipped, never
// asserted unchecked into a panic.
func TestParseReposMalformedNoPanic(t *testing.T) {
	items := []bitbucket.Repository{
		{
			// First item lacks Owner["uuid"] and any Links at all.
			Uuid: "{1}", Slug: "alpha", Description: "a",
		},
		{
			// Links present but "clone" is the wrong type.
			Uuid: "{2}", Slug: "bravo",
			Links: map[string]any{"clone": "not-a-slice"},
		},
		{
			// clone entries are individually malformed except the last.
			Uuid: "{3}", Slug: "charlie",
			Owner: map[string]any{"uuid": "{owner-3}"},
			Links: map[string]any{"clone": []any{
				"not-a-map",
				map[string]any{"name": "ssh"}, // missing href
				map[string]any{"name": "https", "href": 123},            // href wrong type
				map[string]any{"name": "ssh", "href": "git@x:repo.git"}, // good
			}},
		},
	}

	repos, ownerID := parseRepos(items, "acme")

	if len(repos) != 3 {
		t.Fatalf("len(repos) = %d, want 3 (no item dropped)", len(repos))
	}
	// First item has no uuid, so ownerID falls back to the owner name.
	if ownerID != "acme" {
		t.Errorf("ownerID = %q, want fallback %q", ownerID, "acme")
	}
	// Charlie's single valid ssh link is captured; malformed ones are skipped.
	if repos[2].Src != "git@x:repo.git" {
		t.Errorf("repos[2].Src = %q, want %q", repos[2].Src, "git@x:repo.git")
	}
	if repos[2].URL != "" {
		t.Errorf("repos[2].URL = %q, want empty (https href had wrong type)", repos[2].URL)
	}
}

// TestBitbucketCancellationBetweenPages proves paging is driven by the shared
// paginate() loop: a context cancelled while page 1 is being served stops the
// listing before page 2 is ever requested — go-bitbucket's internal
// auto-pager would fetch it regardless.
func TestBitbucketCancellationBetweenPages(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var page2Hits atomic.Int32

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if r.URL.Query().Get("page") == "2" {
				page2Hits.Add(1)
				fmt.Fprint(w, `{"pagelen":1,"page":2,"values":[{"uuid":"{2}","slug":"b","owner":{"uuid":"{o}"}}]}`)
				return
			}
			cancel() // trip cancellation while page 1 is in flight
			fmt.Fprintf(w,
				`{"pagelen":1,"page":1,"next":%q,"values":[{"uuid":"{1}","slug":"a","owner":{"uuid":"{o}"}}]}`,
				server.URL+r.URL.Path+"?page=2")
		},
	))
	defer server.Close()

	provider, err := newBitbucketProvider(Options{Token: "user:pass"})
	if err != nil {
		t.Fatalf("newBitbucketProvider() error = %v", err)
	}
	baseURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("url.Parse: %v", err)
	}
	provider.client.SetApiBaseURL(*baseURL)

	if _, _, err := provider.fetchRepos(ctx, "acme"); !errors.Is(err, context.Canceled) {
		t.Fatalf("fetchRepos error = %v, want context.Canceled", err)
	}
	if n := page2Hits.Load(); n != 0 {
		t.Fatalf("page 2 requested %d times after cancellation, want 0", n)
	}
}
