package providers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/rafi/gits/domain"
)

// giteaFixture serves the org and user repository listings of a Gitea API,
// paginated by `page`/`limit` with a Link header, and records what it saw.
type giteaFixture struct {
	orgs  map[string][]map[string]any
	users map[string][]map[string]any

	mu    sync.Mutex
	paths []string
	auth  []string
}

func (f *giteaFixture) serve(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.paths = append(f.paths, r.URL.Path+"?page="+r.URL.Query().Get("page"))
		f.auth = append(f.auth, r.Header.Get("Authorization"))
		f.mu.Unlock()

		var repos []map[string]any
		var ok bool
		switch parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/"), "/"); {
		case len(parts) == 3 && parts[0] == "orgs" && parts[2] == "repos":
			repos, ok = f.orgs[parts[1]]
		case len(parts) == 3 && parts[0] == "users" && parts[2] == "repos":
			repos, ok = f.users[parts[1]]
		}
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"message":"GetOrgByName","url":"x"}`)
			return
		}

		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		start := min((page-1)*limit, len(repos))
		end := min(start+limit, len(repos))
		if end < len(repos) {
			next := *r.URL
			q := next.Query()
			q.Set("page", strconv.Itoa(page+1))
			next.RawQuery = q.Encode()
			w.Header().Set("Link", fmt.Sprintf(`<http://%s%s>; rel="next"`, r.Host, next.String()))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(repos[start:end])
	}))
	t.Cleanup(server.Close)
	return server
}

// giteaRepoJSON is one listed repository as the API returns it.
func giteaRepoJSON(owner, name string) map[string]any {
	return map[string]any{
		"id":          len(name),
		"name":        name,
		"full_name":   owner + "/" + name,
		"description": "about " + name,
		"owner":       map[string]any{"login": owner},
		"ssh_url":     "git@forge.test:" + owner + "/" + name + ".git",
		"html_url":    "https://forge.test/" + owner + "/" + name,
	}
}

func repoNames(repos []domain.Repository) []string {
	names := make([]string, 0, len(repos))
	for _, r := range repos {
		names = append(names, r.Name)
	}
	return names
}

// TestGiteaOrgAcrossPages proves an organization's listing is walked page by
// page through the Link header, each repository mapped from the API fields.
func TestGiteaOrgAcrossPages(t *testing.T) {
	t.Parallel()

	var repos []map[string]any
	for i := range giteaPageSize + 3 {
		repos = append(repos, giteaRepoJSON("acme", fmt.Sprintf("repo%02d", i)))
	}
	fixture := &giteaFixture{orgs: map[string][]map[string]any{"acme": repos}}
	server := fixture.serve(t)

	provider, err := newGiteaProvider(domain.ProviderGitea, Options{BaseURL: server.URL})
	if err != nil {
		t.Fatalf("newGiteaProvider: %v", err)
	}
	project := &domain.Project{}
	if err := provider.LoadRepos(t.Context(), "acme", project); err != nil {
		t.Fatalf("LoadRepos: %v", err)
	}

	if len(project.Repos) != giteaPageSize+3 {
		t.Fatalf("got %d repos, want %d", len(project.Repos), giteaPageSize+3)
	}
	wantPaths := []string{"/api/v1/orgs/acme/repos?page=1", "/api/v1/orgs/acme/repos?page=2"}
	if !slices.Equal(fixture.paths, wantPaths) {
		t.Errorf("requests = %v, want %v", fixture.paths, wantPaths)
	}
	want := domain.Repository{
		ID:        "6",
		Name:      "repo00",
		Namespace: "acme",
		Src:       "git@forge.test:acme/repo00.git",
		URL:       "https://forge.test/acme/repo00",
		Desc:      "about repo00",
	}
	if got := project.Repos[0]; !reflect.DeepEqual(got, want) {
		t.Errorf("Repos[0] = %+v, want %+v", got, want)
	}
	if project.ID != "acme" || project.SubProjects != nil {
		t.Errorf("project ID = %q, SubProjects = %v; want acme and flat", project.ID, project.SubProjects)
	}
}

// TestGiteaUserFallback proves an owner that is no organization is listed as
// a user.
func TestGiteaUserFallback(t *testing.T) {
	t.Parallel()

	fixture := &giteaFixture{users: map[string][]map[string]any{
		"rafi": {giteaRepoJSON("rafi", "dotfiles")},
	}}
	server := fixture.serve(t)

	provider, err := newGiteaProvider(domain.ProviderForgejo, Options{BaseURL: server.URL})
	if err != nil {
		t.Fatalf("newGiteaProvider: %v", err)
	}
	project := &domain.Project{}
	if err := provider.LoadRepos(t.Context(), "rafi", project); err != nil {
		t.Fatalf("LoadRepos: %v", err)
	}
	if got := repoNames(project.Repos); !slices.Equal(got, []string{"dotfiles"}) {
		t.Errorf("repos = %v, want [dotfiles]", got)
	}
	wantPaths := []string{"/api/v1/orgs/rafi/repos?page=1", "/api/v1/users/rafi/repos?page=1"}
	if !slices.Equal(fixture.paths, wantPaths) {
		t.Errorf("requests = %v, want %v", fixture.paths, wantPaths)
	}
}

// TestGiteaUnknownOwner proves an owner that is neither organization nor
// user is an error naming the owner and the type.
func TestGiteaUnknownOwner(t *testing.T) {
	t.Parallel()

	server := (&giteaFixture{}).serve(t)
	provider, err := newGiteaProvider(domain.ProviderForgejo, Options{BaseURL: server.URL})
	if err != nil {
		t.Fatalf("newGiteaProvider: %v", err)
	}
	err = provider.LoadRepos(t.Context(), "nobody", &domain.Project{})
	if err == nil || !strings.Contains(err.Error(), `"nobody"`) || !strings.Contains(err.Error(), "forgejo") {
		t.Errorf("LoadRepos(unknown) error = %v, want one naming forgejo and the owner", err)
	}
}

// TestGiteaServerError proves a failure other than 404 is reported as is,
// without falling back to a user listing.
func TestGiteaServerError(t *testing.T) {
	t.Parallel()

	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"message":"token does not have required scope"}`)
	}))
	t.Cleanup(server.Close)

	provider, err := newGiteaProvider(domain.ProviderGitea, Options{BaseURL: server.URL})
	if err != nil {
		t.Fatalf("newGiteaProvider: %v", err)
	}
	err = provider.LoadRepos(t.Context(), "acme", &domain.Project{})
	if err == nil || !strings.Contains(err.Error(), "required scope") {
		t.Errorf("LoadRepos error = %v, want the server's message", err)
	}
	if requests != 1 {
		t.Errorf("requests = %d, want 1", requests)
	}
}

// TestGiteaFiltering proves empty repositories are always skipped and
// archived ones only unless settings.includeArchived is set.
func TestGiteaFiltering(t *testing.T) {
	t.Parallel()

	archived := giteaRepoJSON("acme", "old")
	archived["archived"] = true
	empty := giteaRepoJSON("acme", "blank")
	empty["empty"] = true
	fixture := &giteaFixture{orgs: map[string][]map[string]any{
		"acme": {giteaRepoJSON("acme", "live"), archived, empty},
	}}
	server := fixture.serve(t)

	tests := []struct {
		includeArchived bool
		want            []string
	}{
		{false, []string{"live"}},
		{true, []string{"live", "old"}},
	}
	for _, tt := range tests {
		provider, err := newGiteaProvider(domain.ProviderGitea,
			Options{BaseURL: server.URL, IncludeArchived: tt.includeArchived})
		if err != nil {
			t.Fatalf("newGiteaProvider: %v", err)
		}
		project := &domain.Project{}
		if err := provider.LoadRepos(t.Context(), "acme", project); err != nil {
			t.Fatalf("LoadRepos: %v", err)
		}
		if got := repoNames(project.Repos); !slices.Equal(got, tt.want) {
			t.Errorf("includeArchived=%v: repos = %v, want %v", tt.includeArchived, got, tt.want)
		}
	}
}

// TestGiteaToken proves discovery is anonymous without a token and sends a
// resolved one, including one from tokenCommand, which a type whose token is
// optional must still run.
func TestGiteaToken(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		opts     Options
		wantAuth string
	}{
		{"anonymous", Options{}, ""},
		{"token", Options{Token: "s3cret"}, "token s3cret"},
		{"token command", Options{TokenCommand: "echo from-command"}, "token from-command"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fixture := &giteaFixture{orgs: map[string][]map[string]any{"acme": {}}}
			server := fixture.serve(t)
			tt.opts.BaseURL = server.URL

			provider, err := NewGitProvider(t.Context(), domain.ProviderGitea, tt.opts)
			if err != nil {
				t.Fatalf("NewGitProvider: %v", err)
			}
			if err := provider.LoadRepos(t.Context(), "acme", &domain.Project{}); err != nil {
				t.Fatalf("LoadRepos: %v", err)
			}
			if got := fixture.auth; !slices.Equal(got, []string{tt.wantAuth}) {
				t.Errorf("Authorization headers = %q, want [%q]", got, tt.wantAuth)
			}
		})
	}
}

// TestGiteaLoadReposCancelledCtx proves a canceled context aborts discovery
// before any request is sent.
func TestGiteaLoadReposCancelledCtx(t *testing.T) {
	t.Parallel()

	fixture := &giteaFixture{orgs: map[string][]map[string]any{"acme": {}}}
	server := fixture.serve(t)
	provider, err := newGiteaProvider(domain.ProviderGitea, Options{BaseURL: server.URL})
	if err != nil {
		t.Fatalf("newGiteaProvider: %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	err = provider.LoadRepos(ctx, "acme", &domain.Project{})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("LoadRepos(canceled) error = %v, want context.Canceled", err)
	}
	if len(fixture.paths) != 0 {
		t.Errorf("requests = %v, want none", fixture.paths)
	}
}
