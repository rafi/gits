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

// row builds one repository's status as the probe would, bound to its
// repository so the tree lookup finds it.
func row(repo domain.Repository, st repoStatus) *repoStatus {
	st.repo = bulk.Repo{Repository: repo, Path: repo.Name}
	return &st
}

// results wraps statuses as the run hands them to the renderer, under the
// tree the run visited.
func results(project domain.Project, sts ...*repoStatus) bulk.Results[*repoStatus] {
	res := bulk.Results[*repoStatus]{Project: project}
	for _, st := range sts {
		res.Results = append(res.Results, bulk.Result[*repoStatus]{Repo: st.repo, Value: st, Err: st.err})
	}
	return res
}

// renderDoc renders results to JSON and parses the document back.
func renderDoc(t *testing.T, res bulk.Results[*repoStatus], opts Options) map[string]any {
	t.Helper()
	var buf bytes.Buffer
	if err := renderJSON(&buf, res, opts); err != nil {
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
	st := row(okRepo("api"), repoStatus{
		Snapshot: git.Snapshot{
			Branch: "main", Ahead: 4, Behind: 5,
			WorkTree: git.WorkTree{Staged: 1, Unstaged: 2, Untracked: 3},
		},
		compared: true,
		version:  "v1.2.3",
		head:     git.Head{Hash: "abc1234", Subject: "Add feature", Time: when},
	})
	proj := domain.Project{Name: "acme", Repos: []domain.Repository{st.repo.Repository}}

	doc := renderDoc(t, results(proj, st), Options{})
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

	st := row(okRepo("api"), repoStatus{Snapshot: git.Snapshot{Branch: "main"}})
	proj := domain.Project{Name: "acme", Repos: []domain.Repository{st.repo.Repository}}

	doc := renderDoc(t, results(proj, st), Options{})
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
		row(notCloned, repoStatus{err: errors.New("not cloned")}),
		row(broken, repoStatus{err: errors.New("boom")}),
	}
	proj := domain.Project{Name: "acme", Repos: []domain.Repository{notCloned, broken}}

	doc := renderDoc(t, results(proj, sts...), Options{})
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

	st := row(okRepo("api"), repoStatus{err: errors.New("unable to read repo snapshot: boom")})
	proj := domain.Project{Name: "acme", Repos: []domain.Repository{st.repo.Repository}}

	doc := renderDoc(t, results(proj, st), Options{})
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

	proj := domain.Project{Name: "acme", Repos: []domain.Repository{okRepo("api")}}
	measured := row(okRepo("api"), repoStatus{stat: &git.DiffStat{Added: 27, Deleted: 8}})
	unmeasured := row(okRepo("api"), repoStatus{})

	doc := renderDoc(t, results(proj, measured), Options{Stat: true})
	got := reposOf(t, doc["acme"].(map[string]any))[0].(map[string]any)["status"].(map[string]any)
	head, ok := got["head"].(map[string]any)
	if !ok {
		t.Fatalf("no head object under --stat: %v", got)
	}
	if head["added"] != float64(27) || head["deleted"] != float64(8) {
		t.Errorf("head = %v, want +27 -8", head)
	}

	// An unborn HEAD leaves the diff unmeasured: absent, not zero.
	doc = renderDoc(t, results(proj, unmeasured), Options{Stat: true})
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

	doc = renderDoc(t, results(plain, st), Options{})
	got = reposOf(t, doc["acme"].(map[string]any))[0].(map[string]any)["status"].(map[string]any)
	if _, found := got["head"]; found {
		t.Errorf("head emitted without --stat: %v", got)
	}
}

// TestStatusJSONTree: AC-1. Sub-projects nest as the project tree does.
func TestStatusJSONTree(t *testing.T) {
	t.Parallel()

	root := row(okRepo("api"), repoStatus{Snapshot: git.Snapshot{Branch: "main"}})
	nested := row(okRepo("tools"), repoStatus{Snapshot: git.Snapshot{Branch: "main"}})
	sub := domain.Project{Name: "team", Repos: []domain.Repository{nested.repo.Repository}}
	proj := domain.Project{
		Name:        "acme",
		Repos:       []domain.Repository{root.repo.Repository},
		SubProjects: []domain.Project{sub},
	}

	doc := renderDoc(t, results(proj, root, nested), Options{})

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

// TestStatusJSONTreeAtDepth: a node with both repositories and sub-projects
// of its own keeps each repository under the project that owns it, however
// the results happened to arrive.
func TestStatusJSONTreeAtDepth(t *testing.T) {
	t.Parallel()

	// acme ── api
	//   ├── team ── tools
	//   │     └── infra ── ansible
	//   └── vendor ── forks
	leaf := func(name string) *repoStatus {
		return row(okRepo(name), repoStatus{Snapshot: git.Snapshot{Branch: "main"}})
	}
	api, tools, ansible, forks := leaf("api"), leaf("tools"), leaf("ansible"), leaf("forks")

	infra := domain.Project{Name: "infra", Repos: []domain.Repository{ansible.repo.Repository}}
	team := domain.Project{
		Name:        "team",
		Repos:       []domain.Repository{tools.repo.Repository},
		SubProjects: []domain.Project{infra},
	}
	vendor := domain.Project{Name: "vendor", Repos: []domain.Repository{forks.repo.Repository}}
	proj := domain.Project{
		Name:        "acme",
		Repos:       []domain.Repository{api.repo.Repository},
		SubProjects: []domain.Project{team, vendor},
	}

	// Deliberately out of tree order: the document's shape comes from the
	// project tree, not from result order.
	doc := renderDoc(t, results(proj, forks, ansible, api, tools), Options{})

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

	dirty := row(okRepo("api"), repoStatus{Snapshot: git.Snapshot{WorkTree: git.WorkTree{Staged: 1}}})
	clean := row(okRepo("web"), repoStatus{})
	failed := row(okRepo("db"), repoStatus{err: errors.New("boom")})
	tidy := row(okRepo("tidy"), repoStatus{})
	sub := domain.Project{Name: "team", Repos: []domain.Repository{tidy.repo.Repository}}
	proj := domain.Project{
		Name:        "acme",
		Repos:       []domain.Repository{dirty.repo.Repository, clean.repo.Repository, failed.repo.Repository},
		SubProjects: []domain.Project{sub},
	}

	doc := renderDoc(t, results(proj, dirty, clean, failed, tidy), Options{Dirty: true})

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

	clean := row(okRepo("web"), repoStatus{})
	dirty := row(okRepo("tools"), repoStatus{Snapshot: git.Snapshot{WorkTree: git.WorkTree{Staged: 1}}})
	sub := domain.Project{Name: "team", Repos: []domain.Repository{dirty.repo.Repository}}
	proj := domain.Project{
		Name:        "acme",
		Repos:       []domain.Repository{clean.repo.Repository},
		SubProjects: []domain.Project{sub},
	}

	doc := renderDoc(t, results(proj, clean, dirty), Options{Dirty: true})

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

	clean := row(okRepo("web"), repoStatus{})
	proj := domain.Project{Name: "acme", Repos: []domain.Repository{clean.repo.Repository}}

	var buf bytes.Buffer
	if err := renderJSON(&buf, results(proj, clean), Options{Unsynced: true}); err != nil {
		t.Fatalf("renderJSON: %v", err)
	}
	if got := buf.String(); got != "{}\n" {
		t.Errorf("rendered %q, want an empty document", got)
	}
}

// TestStatusJSONSingleRepo: the single-repo form emits the same envelope with
// one repository under its project — the tree the run reports is the project
// narrowed to that repository, so nothing else is documented.
func TestStatusJSONSingleRepo(t *testing.T) {
	t.Parallel()

	st := row(okRepo("api"), repoStatus{Snapshot: git.Snapshot{Branch: "main"}})
	proj := domain.Project{Name: "acme", Repos: []domain.Repository{st.repo.Repository}}

	doc := renderDoc(t, results(proj, st), Options{})
	node := doc["acme"].(map[string]any)
	repos := reposOf(t, node)
	if len(repos) != 1 || repos[0].(map[string]any)["name"] != "api" {
		t.Errorf("repos = %v, want only api", repos)
	}
	if _, found := node["subprojects"]; found {
		t.Error("single-repo output descended into sub-projects")
	}
}

// TestStatusJSONSkipsUnstartedRepos: a repository the run never dequeued
// (Ctrl-C) has no result, and the document holds no entry for it.
func TestStatusJSONSkipsUnstartedRepos(t *testing.T) {
	t.Parallel()

	st := row(okRepo("api"), repoStatus{})
	proj := domain.Project{Name: "acme", Repos: []domain.Repository{st.repo.Repository, okRepo("web")}}

	doc := renderDoc(t, results(proj, st), Options{})
	if repos := reposOf(t, doc["acme"].(map[string]any)); len(repos) != 1 {
		t.Errorf("repos = %v, want only the started one", repos)
	}
}

// TestValidateFormat: AC-8. status takes table and json, and says so instead
// of falling back when handed one of list's other styles. The validator is
// the bulk module's, shared with the line Bulk Commands, and asserted here at
// the seam status reaches it through.
func TestValidateFormat(t *testing.T) {
	t.Parallel()

	for _, format := range []string{"table", "json"} {
		if err := bulk.ValidateFormat(format); err != nil {
			t.Errorf("ValidateFormat(%q) = %v, want nil", format, err)
		}
	}
	for _, format := range []string{"name", "tree", "wide", "", "JSON"} {
		err := bulk.ValidateFormat(format)
		if err == nil {
			t.Errorf("ValidateFormat(%q) = nil, want an error", format)
			continue
		}
		if !strings.Contains(err.Error(), "table") || !strings.Contains(err.Error(), "json") {
			t.Errorf("ValidateFormat(%q) = %q, want it to name the accepted values",
				format, err)
		}
	}
}
