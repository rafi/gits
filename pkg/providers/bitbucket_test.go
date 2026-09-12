package providers

import (
	"testing"

	bitbucket "github.com/ktrysmt/go-bitbucket"
)

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
				map[string]any{"name": "ssh"},                           // missing href
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
