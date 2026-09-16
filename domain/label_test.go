package domain

import (
	"reflect"
	"testing"
)

// TestNewLabelSet checks that comma-separated and repeated values are equal.
func TestNewLabelSet(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		values []string
		want   []Label
	}{
		{"nothing", nil, nil},
		{"one", []string{"demo"}, []Label{"demo"}},
		{"comma-separated", []string{"demo,backend"}, []Label{"backend", "demo"}},
		{"repeated flag", []string{"demo", "backend"}, []Label{"backend", "demo"}},
		{"both forms", []string{"demo,web", "backend"}, []Label{"backend", "demo", "web"}},
		{"folded and trimmed", []string{" Demo , BACKEND "}, []Label{"backend", "demo"}},
		{"deduplicated", []string{"demo,demo", "Demo"}, []Label{"demo"}},
		{"empty values named nothing", []string{"", ",", " "}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := NewLabelSet(tt.values).Names()
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewLabelSet(%q).Names() = %q, want %q", tt.values, got, tt.want)
			}
		})
	}
}

// TestLabelSetComparable checks that equal label sets compare equal with ==.
func TestLabelSetComparable(t *testing.T) {
	t.Parallel()

	if NewLabelSet([]string{"demo,backend"}) != NewLabelSet([]string{"BACKEND", " demo "}) {
		t.Error("two sets naming the same labels are unequal, want normalization to make them equal")
	}
	if NewLabelSet([]string{"demo"}) == NewLabelSet([]string{"backend"}) {
		t.Error("two sets naming different labels are equal")
	}
}

// TestLabelSetEmptyMatchesEverything checks that the zero set matches all.
func TestLabelSetEmptyMatchesEverything(t *testing.T) {
	t.Parallel()

	var none LabelSet
	if !none.Empty() {
		t.Error("the zero LabelSet is not Empty")
	}
	for _, labels := range [][]Label{nil, {"demo"}, {"a", "b"}} {
		if !none.Matches(labels) {
			t.Errorf("empty set does not match %q, want every repository kept", labels)
		}
	}
}

// TestLabelSetMatchesIsUnion checks that a repository matching any label matches.
func TestLabelSetMatchesIsUnion(t *testing.T) {
	t.Parallel()

	set := NewLabelSet([]string{"demo,backend"})
	tests := []struct {
		name   string
		labels []Label
		want   bool
	}{
		{"carries one", []Label{"demo"}, true},
		{"carries the other", []Label{"backend"}, true},
		{"carries both", []Label{"demo", "backend"}, true},
		{"carries one among others", []Label{"web", "backend", "infra"}, true},
		{"carries none", []Label{"web"}, false},
		{"carries nothing at all", nil, false},
		{"a prefix is not a match", []Label{"dem"}, false},
		{"a superstring is not a match", []Label{"demos"}, false},
		{"case is folded", []Label{"DEMO"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := set.Matches(tt.labels); got != tt.want {
				t.Errorf("NewLabelSet(demo,backend).Matches(%q) = %v, want %v",
					tt.labels, got, tt.want)
			}
		})
	}
}

// TestResolveLabels checks that labels are inherited at any depth.
func TestResolveLabels(t *testing.T) {
	t.Parallel()

	project := Project{
		Name:   "root",
		Labels: []Label{"work"},
		Repos:  []Repository{{Name: "api", Labels: []Label{"demo"}}, {Name: "web"}},
		SubProjects: []Project{{
			Name:   "infra",
			Labels: []Label{"ops"},
			Repos:  []Repository{{Name: "db", Labels: []Label{"Demo"}}},
			SubProjects: []Project{{
				Name:  "deep",
				Repos: []Repository{{Name: "cache"}},
			}},
		}},
	}
	project.ResolveLabels(nil)

	want := map[string][]Label{
		"api":   {"demo", "work"},
		"web":   {"work"},
		"db":    {"demo", "ops", "work"},
		"cache": {"ops", "work"},
	}
	for name, wantLabels := range want {
		repo, found := project.GetRepo(name, "")
		if !found {
			t.Fatalf("repository %q missing from the tree", name)
		}
		if !reflect.DeepEqual(repo.Labels, wantLabels) {
			t.Errorf("%s labels = %q, want %q", name, repo.Labels, wantLabels)
		}
	}
}

// TestFilterLabels checks filtering across sub-projects and the match count.
func TestFilterLabels(t *testing.T) {
	t.Parallel()

	newTree := func() Project {
		p := Project{
			Name:  "root",
			Repos: []Repository{{Name: "api", Labels: []Label{"demo"}}, {Name: "web"}},
			SubProjects: []Project{{
				Name:  "infra",
				Repos: []Repository{{Name: "db", Labels: []Label{"demo"}}, {Name: "dns"}},
			}},
		}
		p.ResolveLabels(nil)
		return p
	}

	t.Run("keeps the labeled ones at every depth", func(t *testing.T) {
		t.Parallel()

		p := newTree()
		if kept := p.FilterLabels(NewLabelSet([]string{"demo"})); kept != 2 {
			t.Errorf("FilterLabels kept = %d, want 2", kept)
		}
		if names := p.ListReposWithNamespace(); !reflect.DeepEqual(names, []string{"api", "infra/db"}) {
			t.Errorf("repositories = %q, want api and infra/db", names)
		}
	})

	t.Run("an empty set keeps everything", func(t *testing.T) {
		t.Parallel()

		p := newTree()
		if kept := p.FilterLabels(LabelSet{}); kept != 4 {
			t.Errorf("FilterLabels(empty) kept = %d, want every repository (4)", kept)
		}
	})

	t.Run("a label nothing carries keeps nothing", func(t *testing.T) {
		t.Parallel()

		p := newTree()
		if kept := p.FilterLabels(NewLabelSet([]string{"ghost"})); kept != 0 {
			t.Errorf("FilterLabels(ghost) kept = %d, want 0", kept)
		}
		// Empty sub-projects are kept.
		if len(p.SubProjects) != 1 {
			t.Errorf("sub-projects = %d, want the emptied node kept", len(p.SubProjects))
		}
	})
}

// TestFilterLabelsAfterExclude checks that excluded repositories stay excluded.
func TestFilterLabelsAfterExclude(t *testing.T) {
	t.Parallel()

	p := Project{
		Name:    "root",
		Exclude: []string{"legacy"},
		Repos: []Repository{
			{Name: "api", Labels: []Label{"demo"}},
			{Name: "legacy", Labels: []Label{"demo"}},
		},
	}
	p.ResolveLabels(nil)
	p.Filter()
	p.FilterLabels(NewLabelSet([]string{"demo"}))

	if names := p.ListReposWithNamespace(); !reflect.DeepEqual(names, []string{"api"}) {
		t.Errorf("repositories = %q, want the excluded one to stay excluded", names)
	}
}
