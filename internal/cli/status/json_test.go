package status

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/bulk"
	"github.com/rafi/gits/internal/git"
)

// okRepo is a cloned repository the Traversal would have probed.
func okRepo(name string) domain.Repository {
	return domain.Repository{Name: name, Src: "git@host:acme/" + name + ".git",
		AbsPath: "/code/acme/" + name, State: domain.RepoStateOK}
}

// group wraps statuses as the Traversal hands them to the renderer.
func group(project domain.Project, sts ...*repoStatus) bulk.Group[*repoStatus] {
	g := bulk.Group[*repoStatus]{Project: project}
	for _, st := range sts {
		g.Results = append(g.Results, &bulk.Result[*repoStatus]{Value: st, Err: st.err})
	}
	return g
}

// renderDoc renders groups to JSON and parses the document back.
func renderDoc(t *testing.T, groups []bulk.Group[*repoStatus], tree bool, opts Options) map[string]any {
	t.Helper()
	var buf bytes.Buffer
	if err := renderJSON(&buf, groups, tree, opts); err != nil {
		t.Fatalf("renderJSON: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("unmarshal %s: %v", buf.String(), err)
	}
	return doc
}

// reposOf returns a decoded project's repository objects.
func reposOf(t *testing.T, node map[string]any) []any {
	t.Helper()
	repos, ok := node["repos"].([]any)
	if !ok {
		t.Fatalf("no repos in %v", node)
	}
	return repos
}

// TestStatusJSONNestsStatus: AC-1, AC-4. A cloned repository carries its
// identity, its state, and the working-tree data git was asked for.
func TestStatusJSONNestsStatus(t *testing.T) {
	t.Parallel()

	when := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	st := &repoStatus{
		repo: okRepo("api"), title: "api", branch: "main",
		staged: 1, unstaged: 2, untracked: 3,
		ahead: 4, behind: 5,
		version: "v1.2.3", commit: "abc1234", message: "Add feature", when: when,
	}
	proj := domain.Project{Name: "acme", Repos: []domain.Repository{st.repo}}

	doc := renderDoc(t, []bulk.Group[*repoStatus]{group(proj, st)}, true, Options{})
	node, ok := doc["acme"].(map[string]any)
	if !ok {
		t.Fatalf("project acme missing from %v", doc)
	}
	repo := reposOf(t, node)[0].(map[string]any)
	if repo["name"] != "api" || repo["state"] != "ok" {
		t.Errorf("identity/state = %v", repo)
	}

	got, ok := repo["status"].(map[string]any)
	if !ok {
		t.Fatalf("no status object on an ok repo: %v", repo)
	}
	want := map[string]any{
		"branch": "main", "staged": float64(1), "unstaged": float64(2),
		"untracked": float64(3), "ahead": float64(4), "behind": float64(5),
		"compared": true, "version": "v1.2.3",
	}
	for key, value := range want {
		if got[key] != value {
			t.Errorf("status[%q] = %v, want %v", key, got[key], value)
		}
	}
	commit := got["commit"].(map[string]any)
	if commit["hash"] != "abc1234" || commit["subject"] != "Add feature" {
		t.Errorf("commit = %v", commit)
	}
	if commit["time"] != "2026-08-23T12:00:00Z" {
		t.Errorf("commit time = %v", commit["time"])
	}
}

// TestStatusJSONNoUpstream: an unmeasurable comparison is reported as such
// rather than as zero divergence.
func TestStatusJSONNoUpstream(t *testing.T) {
	t.Parallel()

	st := &repoStatus{repo: okRepo("api"), branch: "main", noUpstream: true}
	proj := domain.Project{Name: "acme", Repos: []domain.Repository{st.repo}}

	doc := renderDoc(t, []bulk.Group[*repoStatus]{group(proj, st)}, true, Options{})
	got := reposOf(t, doc["acme"].(map[string]any))[0].(map[string]any)["status"].(map[string]any)
	if got["compared"] != false {
		t.Errorf("compared = %v, want false", got["compared"])
	}
}

// TestStatusJSONSkipsUnprobed: AC-4. Git is not consulted for a repository
// that isn't cloned, so it carries no status object at all — the state and
// its reason say everything that is known.
func TestStatusJSONSkipsUnprobed(t *testing.T) {
	t.Parallel()

	notCloned := domain.Repository{Name: "web", State: domain.RepoStateNotCloned}
	broken := domain.Repository{
		Name:   "tools",
		State:  domain.RepoStateError,
		Reason: "no `path:` on the project and no `dir:` on the repository",
	}
	sts := []*repoStatus{
		{repo: notCloned, err: errors.New("not cloned"), message: "not cloned"},
		{repo: broken, err: errors.New("boom"), message: "boom"},
	}
	proj := domain.Project{Name: "acme", Repos: []domain.Repository{notCloned, broken}}

	doc := renderDoc(t, []bulk.Group[*repoStatus]{group(proj, sts...)}, true, Options{})
	repos := reposOf(t, doc["acme"].(map[string]any))

	web := repos[0].(map[string]any)
	if web["state"] != "not-cloned" {
		t.Errorf("state = %v, want not-cloned", web["state"])
	}
	if _, found := web["status"]; found {
		t.Error("status object emitted for a repo git never saw")
	}
	if _, found := web["reason"]; found {
		t.Error("reason emitted for a not-cloned repo")
	}

	tools := repos[1].(map[string]any)
	if tools["state"] != "error" ||
		tools["reason"] != "no `path:` on the project and no `dir:` on the repository" {
		t.Errorf("error repo = %v", tools)
	}
	if _, found := tools["status"]; found {
		t.Error("status object emitted for an error repo")
	}
}

// TestStatusJSONProbeError: AC-5. A failed probe emits only its reason; the
// counts were never measured.
func TestStatusJSONProbeError(t *testing.T) {
	t.Parallel()

	st := &repoStatus{
		repo: okRepo("api"),
		err:  errors.New("unable to read repo snapshot: boom"),
		// The table's message cell carries the bare reason.
		message: "unable to read repo snapshot: boom",
	}
	proj := domain.Project{Name: "acme", Repos: []domain.Repository{st.repo}}

	doc := renderDoc(t, []bulk.Group[*repoStatus]{group(proj, st)}, true, Options{})
	repo := reposOf(t, doc["acme"].(map[string]any))[0].(map[string]any)
	got := repo["status"].(map[string]any)
	if len(got) != 1 || got["error"] != "unable to read repo snapshot: boom" {
		t.Errorf("status = %v, want only an error key", got)
	}
}

// TestStatusJSONHead: AC-6. The HEAD± counts appear only when --stat asked
// for them and the diff probe actually answered.
func TestStatusJSONHead(t *testing.T) {
	t.Parallel()

	proj := domain.Project{Name: "acme"}
	measured := &repoStatus{repo: okRepo("api"), added: 27, deleted: 8, hasStat: true}
	unmeasured := &repoStatus{repo: okRepo("api")}

	doc := renderDoc(t, []bulk.Group[*repoStatus]{group(proj, measured)}, true, Options{Stat: true})
	got := reposOf(t, doc["acme"].(map[string]any))[0].(map[string]any)["status"].(map[string]any)
	head, ok := got["head"].(map[string]any)
	if !ok {
		t.Fatalf("no head object under --stat: %v", got)
	}
	if head["added"] != float64(27) || head["deleted"] != float64(8) {
		t.Errorf("head = %v, want +27 -8", head)
	}

	// An unborn HEAD leaves the diff unmeasured: absent, not zero.
	doc = renderDoc(t, []bulk.Group[*repoStatus]{group(proj, unmeasured)}, true, Options{Stat: true})
	got = reposOf(t, doc["acme"].(map[string]any))[0].(map[string]any)["status"].(map[string]any)
	if _, found := got["head"]; found {
		t.Errorf("head emitted for an unmeasured diff: %v", got)
	}

	// Without --stat the diff is never probed at all. Run the real prober so
	// this asserts the whole chain rather than a hand-built status that the
	// prober could not have produced.
	deps := statusDeps(t, fakeGit{
		workingDiff: git.DiffStat{Added: 27, Deleted: 8},
		snap:        git.Snapshot{Branch: "main", Tracking: true},
	})
	repo := okRepo("api")
	plain := domain.Project{Name: "acme", Repos: []domain.Repository{repo}}
	st, err := probe(t, deps, Options{}, plain, repo)
	if err != nil {
		t.Fatalf("probe: %v", err)
	}

	doc = renderDoc(t, []bulk.Group[*repoStatus]{
		{Project: plain, Results: []*bulk.Result[*repoStatus]{{Value: st}}},
	}, true, Options{})
	got = reposOf(t, doc["acme"].(map[string]any))[0].(map[string]any)["status"].(map[string]any)
	if _, found := got["head"]; found {
		t.Errorf("head emitted without --stat: %v", got)
	}
}

// TestStatusJSONTree: AC-1. Sub-projects nest, rebuilt from the Traversal's
// depth-first group order.
func TestStatusJSONTree(t *testing.T) {
	t.Parallel()

	root := &repoStatus{repo: okRepo("api"), branch: "main"}
	nested := &repoStatus{repo: okRepo("tools"), branch: "main"}
	sub := domain.Project{Name: "team", Repos: []domain.Repository{nested.repo}}
	proj := domain.Project{
		Name:        "acme",
		Repos:       []domain.Repository{root.repo},
		SubProjects: []domain.Project{sub},
	}

	doc := renderDoc(t, []bulk.Group[*repoStatus]{
		group(proj, root),
		group(sub, nested),
	}, true, Options{})

	node := doc["acme"].(map[string]any)
	if got := reposOf(t, node)[0].(map[string]any)["name"]; got != "api" {
		t.Errorf("root repo = %v, want api", got)
	}
	subs, ok := node["subprojects"].([]any)
	if !ok || len(subs) != 1 {
		t.Fatalf("sub-projects = %v", node["subprojects"])
	}
	subNode := subs[0].(map[string]any)
	if subNode["name"] != "team" {
		t.Errorf("sub-project name = %v", subNode["name"])
	}
	if got := reposOf(t, subNode)[0].(map[string]any)["name"]; got != "tools" {
		t.Errorf("sub-project repo = %v, want tools", got)
	}
}

// TestStatusJSONTreeAtDepth: the inversion has to survive a node that has both
// repositories and sub-projects of its own — the case where miscounting the
// groups a subtree consumed would reparent everything after it.
func TestStatusJSONTreeAtDepth(t *testing.T) {
	t.Parallel()

	// acme ── api
	//   ├── team ── tools
	//   │     └── infra ── ansible
	//   └── vendor ── forks
	leaf := func(name string) *repoStatus {
		return &repoStatus{repo: okRepo(name), branch: "main"}
	}
	api, tools, ansible, forks := leaf("api"), leaf("tools"), leaf("ansible"), leaf("forks")

	infra := domain.Project{Name: "infra", Repos: []domain.Repository{ansible.repo}}
	team := domain.Project{
		Name:        "team",
		Repos:       []domain.Repository{tools.repo},
		SubProjects: []domain.Project{infra},
	}
	vendor := domain.Project{Name: "vendor", Repos: []domain.Repository{forks.repo}}
	proj := domain.Project{
		Name:        "acme",
		Repos:       []domain.Repository{api.repo},
		SubProjects: []domain.Project{team, vendor},
	}

	// The Traversal's order: a project's repositories, then each sub-project's
	// subtree.
	doc := renderDoc(t, []bulk.Group[*repoStatus]{
		group(proj, api),
		group(team, tools),
		group(infra, ansible),
		group(vendor, forks),
	}, true, Options{})

	// path walks down named sub-projects and returns the node's repo names.
	path := func(node map[string]any, names ...string) []string {
		for _, name := range names {
			subs, ok := node["subprojects"].([]any)
			if !ok {
				t.Fatalf("no sub-projects under %v, looking for %q", node["name"], name)
			}
			found := false
			for _, sub := range subs {
				if s := sub.(map[string]any); s["name"] == name {
					node, found = s, true
					break
				}
			}
			if !found {
				t.Fatalf("sub-project %q missing from %v", name, subs)
			}
		}
		var got []string
		for _, repo := range reposOf(t, node) {
			got = append(got, repo.(map[string]any)["name"].(string))
		}
		return got
	}

	root := doc["acme"].(map[string]any)
	for _, tc := range []struct {
		want string
		at   []string
	}{
		{"api", nil},
		{"tools", []string{"team"}},
		{"ansible", []string{"team", "infra"}},
		{"forks", []string{"vendor"}},
	} {
		got := path(root, tc.at...)
		if len(got) != 1 || got[0] != tc.want {
			t.Errorf("repos at %v = %v, want [%s]", tc.at, got, tc.want)
		}
	}
}

// TestStatusJSONFilters: AC-7. --dirty drops clean rows the way it drops
// table rows, keeps error rows, and takes emptied project nodes with it.
func TestStatusJSONFilters(t *testing.T) {
	t.Parallel()

	dirty := &repoStatus{repo: okRepo("api"), staged: 1}
	clean := &repoStatus{repo: okRepo("web")}
	failed := &repoStatus{repo: okRepo("db"), err: errors.New("boom"), message: "boom"}
	sub := domain.Project{Name: "team", Repos: []domain.Repository{clean.repo}}
	proj := domain.Project{
		Name:        "acme",
		Repos:       []domain.Repository{dirty.repo, clean.repo, failed.repo},
		SubProjects: []domain.Project{sub},
	}

	doc := renderDoc(t, []bulk.Group[*repoStatus]{
		group(proj, dirty, clean, failed),
		group(sub, clean),
	}, true, Options{Dirty: true})

	node := doc["acme"].(map[string]any)
	repos := reposOf(t, node)
	if len(repos) != 2 {
		t.Fatalf("kept %d repos, want the dirty one and the failed one: %v", len(repos), repos)
	}
	names := []string{
		repos[0].(map[string]any)["name"].(string),
		repos[1].(map[string]any)["name"].(string),
	}
	if names[0] != "api" || names[1] != "db" {
		t.Errorf("kept %v, want [api db]", names)
	}
	if _, found := node["subprojects"]; found {
		t.Errorf("sub-project with nothing left in it survived: %v", node["subprojects"])
	}
}

// TestStatusJSONFilterKeepsBridgingParent: AC-7, and the one place the tree
// cannot match the table. A parent whose own repositories were all filtered
// out stays when a sub-project survived — the survivor has nowhere else to
// hang. The table, whose groups are flat, drops that header instead.
func TestStatusJSONFilterKeepsBridgingParent(t *testing.T) {
	t.Parallel()

	clean := &repoStatus{repo: okRepo("web")}
	dirty := &repoStatus{repo: okRepo("tools"), staged: 1}
	sub := domain.Project{Name: "team", Repos: []domain.Repository{dirty.repo}}
	proj := domain.Project{
		Name:        "acme",
		Repos:       []domain.Repository{clean.repo},
		SubProjects: []domain.Project{sub},
	}

	doc := renderDoc(t, []bulk.Group[*repoStatus]{
		group(proj, clean),
		group(sub, dirty),
	}, true, Options{Dirty: true})

	node, ok := doc["acme"].(map[string]any)
	if !ok {
		t.Fatalf("the bridging parent was dropped, orphaning its sub-project: %v", doc)
	}
	if _, found := node["repos"]; found {
		t.Errorf("parent kept repos its filter hid: %v", node["repos"])
	}
	subs := node["subprojects"].([]any)
	if len(subs) != 1 || subs[0].(map[string]any)["name"] != "team" {
		t.Errorf("sub-projects = %v, want just team", subs)
	}
}

// TestStatusJSONFilterEmptiesEverything: a filter that matches nothing leaves
// an empty document rather than a project full of nothing.
func TestStatusJSONFilterEmptiesEverything(t *testing.T) {
	t.Parallel()

	clean := &repoStatus{repo: okRepo("web")}
	proj := domain.Project{Name: "acme", Repos: []domain.Repository{clean.repo}}

	var buf bytes.Buffer
	if err := renderJSON(&buf, []bulk.Group[*repoStatus]{group(proj, clean)}, true,
		Options{Unsynced: true}); err != nil {
		t.Fatalf("renderJSON: %v", err)
	}
	if got := buf.String(); got != "{}\n" {
		t.Errorf("rendered %q, want an empty document", got)
	}
}

// TestStatusJSONSingleRepo: the single-repo form emits the same envelope with
// one repository under its project, not the project's whole tree.
func TestStatusJSONSingleRepo(t *testing.T) {
	t.Parallel()

	st := &repoStatus{repo: okRepo("api"), branch: "main"}
	proj := domain.Project{
		Name:        "acme",
		Repos:       []domain.Repository{st.repo, okRepo("web")},
		SubProjects: []domain.Project{{Name: "team"}},
	}

	doc := renderDoc(t, []bulk.Group[*repoStatus]{group(proj, st)}, false, Options{})
	node := doc["acme"].(map[string]any)
	repos := reposOf(t, node)
	if len(repos) != 1 || repos[0].(map[string]any)["name"] != "api" {
		t.Errorf("repos = %v, want only api", repos)
	}
	if _, found := node["subprojects"]; found {
		t.Error("single-repo output descended into sub-projects")
	}
}

// TestStatusJSONSkipsUnstartedRepos: a repository the Traversal never dequeued
// (Ctrl-C) leaves a nil slot, which must not become a null entry in the
// document.
func TestStatusJSONSkipsUnstartedRepos(t *testing.T) {
	t.Parallel()

	st := &repoStatus{repo: okRepo("api")}
	proj := domain.Project{Name: "acme", Repos: []domain.Repository{st.repo}}
	g := group(proj, st)
	g.Results = append(g.Results, nil)

	doc := renderDoc(t, []bulk.Group[*repoStatus]{g}, true, Options{})
	if repos := reposOf(t, doc["acme"].(map[string]any)); len(repos) != 1 {
		t.Errorf("repos = %v, want only the started one", repos)
	}
}

// TestValidateFormat: AC-8. status takes table and json, and says so instead
// of falling back when handed one of list's other styles.
func TestValidateFormat(t *testing.T) {
	t.Parallel()

	for _, format := range []string{"table", "json"} {
		if err := validateFormat(format); err != nil {
			t.Errorf("validateFormat(%q) = %v, want nil", format, err)
		}
	}
	for _, format := range []string{"name", "tree", "wide", "", "JSON"} {
		err := validateFormat(format)
		if err == nil {
			t.Errorf("validateFormat(%q) = nil, want an error", format)
			continue
		}
		if !strings.Contains(err.Error(), "table") || !strings.Contains(err.Error(), "json") {
			t.Errorf("validateFormat(%q) = %q, want it to name the accepted values",
				format, err)
		}
	}
}
