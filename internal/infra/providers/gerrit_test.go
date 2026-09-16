package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
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

// gerritMagicPrefix guards every Gerrit JSON response against XSSI.
const gerritMagicPrefix = ")]}'\n"

// gerritFixture serves a Gerrit REST API's project listing, filtered by `p`
// and paged by `n`/`S` with `_more_projects`, and its server info, and
// records what it saw.
type gerritFixture struct {
	// projects are listed in name order, as Gerrit does.
	projects map[string]map[string]any
	// schemes are the download schemes server info advertises.
	schemes map[string]any

	mu    sync.Mutex
	paths []string
	auth  []string
}

func (f *gerritFixture) serve(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.paths = append(f.paths, r.URL.RequestURI())
		f.auth = append(f.auth, r.Header.Get("Authorization"))
		f.mu.Unlock()

		path := strings.TrimPrefix(r.URL.Path, "/a")
		w.Header().Set("Content-Type", "application/json")
		switch path {
		case "/config/server/info/", "/config/server/info":
			fmt.Fprint(w, gerritMagicPrefix)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"download": map[string]any{"schemes": f.schemes},
			})
		case "/projects/":
			f.listProjects(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func (f *gerritFixture) listProjects(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var names []string
	for name := range f.projects {
		if strings.HasPrefix(name, q.Get("p")) {
			names = append(names, name)
		}
	}
	slices.Sort(names)

	skip, _ := strconv.Atoi(q.Get("S"))
	limit, _ := strconv.Atoi(q.Get("n"))
	start := min(skip, len(names))
	end := len(names)
	if limit > 0 {
		end = min(start+limit, len(names))
	}
	page := map[string]map[string]any{}
	for i, name := range names[start:end] {
		info := map[string]any{"id": strings.ReplaceAll(name, "/", "%2F")}
		maps.Copy(info, f.projects[name])
		if start+i == end-1 && end < len(names) {
			info["_more_projects"] = true
		}
		page[name] = info
	}
	fmt.Fprint(w, gerritMagicPrefix)
	_ = json.NewEncoder(w).Encode(page)
}

// gerritSSHScheme is the download scheme of a server cloning over SSH.
var gerritSSHScheme = map[string]any{
	"ssh":  map[string]any{"url": "ssh://review.test:29418/${project}"},
	"http": map[string]any{"url": "https://review.test/${project}"},
}

func activeProjects(names ...string) map[string]map[string]any {
	projects := map[string]map[string]any{}
	for _, name := range names {
		projects[name] = map[string]any{"state": "ACTIVE", "description": "about " + name}
	}
	return projects
}

func loadGerrit(t *testing.T, opts Options, prefix string) *domain.Project {
	t.Helper()
	provider, err := newGerritProvider(opts)
	if err != nil {
		t.Fatalf("newGerritProvider: %v", err)
	}
	project := &domain.Project{}
	if err := provider.LoadRepos(t.Context(), prefix, project); err != nil {
		t.Fatalf("LoadRepos: %v", err)
	}
	return project
}

// TestGerritPagesAcrossMoreProjects proves the listing is walked with `S`
// until no entry carries `_more_projects`.
func TestGerritPagesAcrossMoreProjects(t *testing.T) {
	t.Parallel()

	var names []string
	for i := range gerritPageSize + 2 {
		names = append(names, fmt.Sprintf("p/repo%03d", i))
	}
	fixture := &gerritFixture{projects: activeProjects(names...), schemes: gerritSSHScheme}
	server := fixture.serve(t)

	project := loadGerrit(t, Options{BaseURL: server.URL}, "p/")

	if got := repoNames(project.Repos); len(got) != len(names) {
		t.Fatalf("got %d repos, want %d", len(got), len(names))
	}
	var listings []string
	for _, p := range fixture.paths {
		if strings.HasPrefix(p, "/projects/") {
			listings = append(listings, p)
		}
	}
	n := strconv.Itoa(gerritPageSize)
	want := []string{
		"/projects/?d=true&n=" + n + "&p=p%2F",
		"/projects/?S=" + n + "&d=true&n=" + n + "&p=p%2F",
	}
	if !slices.Equal(listings, want) {
		t.Errorf("listings = %v, want %v", listings, want)
	}
}

// TestGerritFiltersProjects proves hidden and meta projects are never listed
// and read-only ones count as archived.
func TestGerritFiltersProjects(t *testing.T) {
	t.Parallel()

	projects := activeProjects("All-Projects", "All-Users", "live")
	projects["gone"] = map[string]any{"state": "HIDDEN"}
	projects["frozen"] = map[string]any{"state": "READ_ONLY"}
	server := (&gerritFixture{projects: projects, schemes: gerritSSHScheme}).serve(t)

	tests := []struct {
		name            string
		includeArchived bool
		want            []string
	}{
		{"archived excluded", false, []string{"live"}},
		{"archived included", true, []string{"frozen", "live"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			project := loadGerrit(t, Options{BaseURL: server.URL, IncludeArchived: tt.includeArchived}, "")
			if got := repoNames(project.Repos); !slices.Equal(got, tt.want) {
				t.Errorf("repos = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestGerritTreeBelowPrefix proves `/` segments become Sub-projects rooted at
// the last complete segment of the prefix, each repository mapped in full.
func TestGerritTreeBelowPrefix(t *testing.T) {
	t.Parallel()

	fixture := &gerritFixture{
		projects: activeProjects("openstack/nova", "openstack/oslo/db", "openstack/oslo/config", "starlingx/root"),
		schemes:  gerritSSHScheme,
	}
	server := fixture.serve(t)

	project := loadGerrit(t, Options{BaseURL: server.URL}, "openstack/")

	if project.ID != "openstack/" {
		t.Errorf("project ID = %q, want openstack/", project.ID)
	}
	want := domain.Repository{
		ID:        "openstack/nova",
		Name:      "nova",
		Namespace: "openstack",
		Src:       "ssh://review.test:29418/openstack/nova",
		URL:       server.URL + "/admin/repos/openstack/nova",
		Desc:      "about openstack/nova",
	}
	if len(project.Repos) != 1 || !reflect.DeepEqual(project.Repos[0], want) {
		t.Errorf("root repos = %+v, want [%+v]", project.Repos, want)
	}
	if len(project.SubProjects) != 1 {
		t.Fatalf("sub-projects = %+v, want one (oslo)", project.SubProjects)
	}
	oslo := project.SubProjects[0]
	if oslo.ID != "openstack/oslo" || oslo.Name != "oslo" {
		t.Errorf("sub-project = %q (%q), want oslo (openstack/oslo)", oslo.Name, oslo.ID)
	}
	if got := repoNames(oslo.Repos); !slices.Equal(got, []string{"config", "db"}) {
		t.Errorf("oslo repos = %v, want [config db]", got)
	}

	// A prefix ending mid-segment roots the tree above it.
	project = loadGerrit(t, Options{BaseURL: server.URL}, "open")
	if len(project.Repos) != 0 || len(project.SubProjects) != 1 || project.SubProjects[0].Name != "openstack" {
		t.Errorf("prefix open: repos %v, sub-projects %+v; want only openstack", project.Repos, project.SubProjects)
	}
}

// TestGerritSrc proves clone URLs come from the server's SSH scheme with the
// username, or its HTTP scheme when it advertises no SSH.
func TestGerritSrc(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		username string
		schemes  map[string]any
		want     string
	}{
		{"ssh without username", "", gerritSSHScheme, "ssh://review.test:29418/team/app"},
		{"ssh with username", "rafi", gerritSSHScheme, "ssh://rafi@review.test:29418/team/app"},
		{
			"no ssh scheme", "rafi",
			map[string]any{"anonymous http": map[string]any{"url": "https://review.test/r/${project}"}},
			"https://review.test/r/team/app",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server := (&gerritFixture{projects: activeProjects("team/app"), schemes: tt.schemes}).serve(t)
			project := loadGerrit(t, Options{BaseURL: server.URL, Username: tt.username}, "team/")
			if len(project.Repos) != 1 || project.Repos[0].Src != tt.want {
				t.Errorf("repos = %+v, want Src %q", project.Repos, tt.want)
			}
		})
	}
}

// TestGerritAuth proves discovery is anonymous without a token, and sends
// basic auth under /a/ with one.
func TestGerritAuth(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		opts       Options
		wantPrefix string
		wantAuth   string
	}{
		{"anonymous", Options{Username: "rafi"}, "/projects/", ""},
		{"token", Options{Username: "rafi", Token: "secret"}, "/a/projects/", "Basic cmFmaTpzZWNyZXQ="},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fixture := &gerritFixture{projects: activeProjects("x"), schemes: gerritSSHScheme}
			tt.opts.BaseURL = fixture.serve(t).URL
			loadGerrit(t, tt.opts, "")

			for i, path := range fixture.paths {
				if strings.HasPrefix(path, "/a/") != strings.HasPrefix(tt.wantPrefix, "/a/") {
					t.Errorf("request %s, want every request under %q", path, tt.wantPrefix)
				}
				if fixture.auth[i] != tt.wantAuth {
					t.Errorf("request %s Authorization = %q, want %q", path, fixture.auth[i], tt.wantAuth)
				}
			}
			if !slices.ContainsFunc(fixture.paths, func(p string) bool { return strings.HasPrefix(p, tt.wantPrefix) }) {
				t.Errorf("requests = %v, want a listing under %q", fixture.paths, tt.wantPrefix)
			}
		})
	}
}

// TestGerritTokenNeedsUsername proves a token without a username to send it
// as is rejected before any request.
func TestGerritTokenNeedsUsername(t *testing.T) {
	t.Parallel()

	_, err := newGerritProvider(Options{BaseURL: "https://review.test", Token: "secret"})
	if err == nil || !strings.Contains(err.Error(), "username") {
		t.Errorf("newGerritProvider(token, no username) error = %v, want one naming username", err)
	}
	if err != nil && strings.Contains(err.Error(), "secret") {
		t.Errorf("error leaks the token: %v", err)
	}
}

// TestGerritCanceled proves a canceled context stops discovery.
func TestGerritCanceled(t *testing.T) {
	t.Parallel()

	server := (&gerritFixture{projects: activeProjects("x"), schemes: gerritSSHScheme}).serve(t)
	provider, err := newGerritProvider(Options{BaseURL: server.URL})
	if err != nil {
		t.Fatalf("newGerritProvider: %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := provider.LoadRepos(ctx, "", &domain.Project{}); err == nil {
		t.Error("LoadRepos(canceled) = nil, want error")
	}
}
