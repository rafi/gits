package jsonout

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/rafi/gits/domain"
)

// decode marshals v through Write and parses the result back, so every test
// asserts on the bytes a user would actually receive.
func decode(t *testing.T, env Envelope) map[string]any {
	t.Helper()
	var buf bytes.Buffer
	if err := Write(&buf, env); err != nil {
		t.Fatalf("Write: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal %s: %v", buf.String(), err)
	}
	return got
}

// repoOf digs one repository object out of the decoded envelope's only
// project, which every test here names "acme".
func repoOf(t *testing.T, doc map[string]any, idx int) map[string]any {
	t.Helper()
	const project = "acme"
	proj, ok := doc[project].(map[string]any)
	if !ok {
		t.Fatalf("project %q missing from %v", project, doc)
	}
	repos, ok := proj["repos"].([]any)
	if !ok || len(repos) <= idx {
		t.Fatalf("project %q has no repo %d: %v", project, idx, proj)
	}
	return repos[idx].(map[string]any)
}

// TestEnvelopeCarriesState: AC-3. Every repository reports its Repo State,
// the field domain.Repository deliberately hides from the cache.
func TestEnvelopeCarriesState(t *testing.T) {
	t.Parallel()

	env := FromProjects(domain.ProjectListKeyed{
		"acme": {
			Name: "acme",
			Repos: []domain.Repository{
				{Name: "api", Src: "git@host:acme/api.git", State: domain.RepoStateOK},
			},
		},
	})

	repo := repoOf(t, decode(t, env), 0)
	if repo["state"] != "ok" {
		t.Errorf("state = %v, want %q", repo["state"], "ok")
	}
	if _, found := repo["reason"]; found {
		t.Errorf("reason emitted for a non-error repo: %v", repo["reason"])
	}
	if _, found := repo["status"]; found {
		t.Error("list envelope emitted a status object")
	}
}

// TestEnvelopeCarriesReason: AC-3. The error state carries its Reason, and no
// other state does.
func TestEnvelopeCarriesReason(t *testing.T) {
	t.Parallel()

	env := FromProjects(domain.ProjectListKeyed{
		"acme": {
			Name: "acme",
			Repos: []domain.Repository{
				{Name: "api", State: domain.RepoStateError, Reason: "no `path:` on the project"},
				{Name: "web", State: domain.RepoStateNotCloned, Reason: "leftover"},
			},
		},
	})

	doc := decode(t, env)
	if got := repoOf(t, doc, 0)["reason"]; got != "no `path:` on the project" {
		t.Errorf("reason = %v, want the classification reason", got)
	}
	if _, found := repoOf(t, doc, 1)["reason"]; found {
		t.Error("reason emitted for a not-cloned repo")
	}
}

// TestEnvelopeKeepsListShape: AC-2. The wire types mirror the fields
// list -o json already shipped, sub-projects included.
func TestEnvelopeKeepsListShape(t *testing.T) {
	t.Parallel()

	env := FromProjects(domain.ProjectListKeyed{
		"acme": {
			ID:     "1",
			Name:   "acme",
			Path:   "~/code/acme",
			Desc:   "widgets",
			Source: &domain.ProviderSource{Type: "github", Search: "acme"},
			Repos: []domain.Repository{{
				ID:        "r1",
				Name:      "api",
				Namespace: "acme",
				Src:       "git@host:acme/api.git",
				Dir:       "api",
				URL:       "https://host/acme/api",
				Desc:      "the api",
				State:     domain.RepoStateOK,
			}},
			SubProjects: []domain.Project{{
				Name:  "team",
				Repos: []domain.Repository{{Name: "tools", State: domain.RepoStateNotCloned}},
			}},
		},
	})

	doc := decode(t, env)
	proj := doc["acme"].(map[string]any)
	for _, key := range []string{"id", "name", "path", "desc", "source", "repos", "subprojects"} {
		if _, found := proj[key]; !found {
			t.Errorf("project key %q missing from %v", key, proj)
		}
	}
	repo := repoOf(t, doc, 0)
	for _, key := range []string{"id", "name", "namespace", "src", "dir", "url", "desc", "state"} {
		if _, found := repo[key]; !found {
			t.Errorf("repo key %q missing from %v", key, repo)
		}
	}

	subs := proj["subprojects"].([]any)
	sub := subs[0].(map[string]any)
	subRepo := sub["repos"].([]any)[0].(map[string]any)
	if subRepo["state"] != "not-cloned" {
		t.Errorf("sub-project repo state = %v, want %q", subRepo["state"], "not-cloned")
	}
}

// TestStatusMarshalMeasured: a successful probe emits every count, zeroes
// included — a clean work tree must be distinguishable from an unmeasured one.
func TestStatusMarshalMeasured(t *testing.T) {
	t.Parallel()

	raw, err := json.Marshal(Status{Branch: "main", Compared: true})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"branch", "staged", "unstaged", "untracked", "ahead", "behind", "compared"} {
		if _, found := got[key]; !found {
			t.Errorf("status key %q missing from %s", key, raw)
		}
	}
	if _, found := got["error"]; found {
		t.Error("error emitted for a successful probe")
	}
	if _, found := got["head"]; found {
		t.Error("head emitted without --stat")
	}
	if _, found := got["upstream"]; found {
		t.Error("upstream emitted for a branch with none: absence is what says so")
	}
}

// TestStatusMarshalUpstream: the nested upstream object carries the Upstream's
// name and whether a ref still resolves behind it, so a Gone Upstream is
// distinguishable from a healthy one without a sentinel value.
func TestStatusMarshalUpstream(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		up   Upstream
	}{
		{"tracked", Upstream{Name: "origin/main", Tracked: true}},
		{"gone", Upstream{Name: "origin/feat-b"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			raw, err := json.Marshal(Status{Branch: "main", Upstream: &tc.up})
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			var got map[string]any
			if err := json.Unmarshal(raw, &got); err != nil {
				t.Fatal(err)
			}
			up, ok := got["upstream"].(map[string]any)
			if !ok {
				t.Fatalf("no upstream object in %s", raw)
			}
			if up["name"] != tc.up.Name || up["tracked"] != tc.up.Tracked {
				t.Errorf("upstream = %v, want %+v", up, tc.up)
			}
		})
	}
}

// TestStatusMarshalError: AC-5. A failed probe measured nothing, so it emits
// nothing but its error — zero counts beside it would read as a clean tree.
func TestStatusMarshalError(t *testing.T) {
	t.Parallel()

	raw, err := json.Marshal(Status{Error: "unable to read repo snapshot: boom"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got["error"] != "unable to read repo snapshot: boom" {
		t.Errorf("failed probe = %s, want only an error key", raw)
	}
}

// TestStatusNested: the status object attaches under the repository, beside
// the identity fields rather than replacing them.
func TestStatusNested(t *testing.T) {
	t.Parallel()

	when := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	repo := NewRepository(domain.Repository{Name: "api", State: domain.RepoStateOK})
	repo.Status = &Status{
		Branch:  "main",
		Staged:  2,
		Head:    &Head{Added: 27, Deleted: 8},
		Commit:  &Commit{Hash: "abc1234", Subject: "Add feature", Time: when},
		Version: "v1.2.3",
	}

	raw, err := json.Marshal(repo)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got["name"] != "api" || got["state"] != "ok" {
		t.Errorf("identity lost: %s", raw)
	}
	st := got["status"].(map[string]any)
	if st["branch"] != "main" || st["staged"] != float64(2) || st["version"] != "v1.2.3" {
		t.Errorf("status = %v", st)
	}
	if head := st["head"].(map[string]any); head["added"] != float64(27) {
		t.Errorf("head = %v", head)
	}
	if commit := st["commit"].(map[string]any); commit["hash"] != "abc1234" {
		t.Errorf("commit = %v", commit)
	}
}

// TestWriteEndsWithNewline: the document is a line on stdout, so a shell
// prompt or a following writer starts clean.
func TestWriteEndsWithNewline(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	if err := Write(&buf, Envelope{}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if got := buf.String(); got != "{}\n" {
		t.Errorf("Write(empty) = %q, want %q", got, "{}\n")
	}
}
