package domain

import (
	"reflect"
	"testing"
)

// TestNewTagSet checks that comma-separated and repeated values are equal.
func TestNewTagSet(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		values []string
		want   []Tag
	}{
		{"nothing", nil, nil},
		{"one", []string{"demo"}, []Tag{"demo"}},
		{"comma-separated", []string{"demo,backend"}, []Tag{"backend", "demo"}},
		{"repeated flag", []string{"demo", "backend"}, []Tag{"backend", "demo"}},
		{"both forms", []string{"demo,web", "backend"}, []Tag{"backend", "demo", "web"}},
		{"folded and trimmed", []string{" Demo , BACKEND "}, []Tag{"backend", "demo"}},
		{"deduplicated", []string{"demo,demo", "Demo"}, []Tag{"demo"}},
		{"empty values named nothing", []string{"", ",", " "}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := NewTagSet(tt.values).Names()
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewTagSet(%q).Names() = %q, want %q", tt.values, got, tt.want)
			}
		})
	}
}

// TestTagSetComparable checks that equal tag sets compare equal with ==.
func TestTagSetComparable(t *testing.T) {
	t.Parallel()

	if NewTagSet([]string{"demo,backend"}) != NewTagSet([]string{"BACKEND", " demo "}) {
		t.Error("two sets naming the same tags are unequal, want normalization to make them equal")
	}
	if NewTagSet([]string{"demo"}) == NewTagSet([]string{"backend"}) {
		t.Error("two sets naming different tags are equal")
	}
}

// TestTagSetEmptyMatchesEverything checks that the zero set matches all.
func TestTagSetEmptyMatchesEverything(t *testing.T) {
	t.Parallel()

	var none TagSet
	if !none.Empty() {
		t.Error("the zero TagSet is not Empty")
	}
	for _, tags := range [][]Tag{nil, {"demo"}, {"a", "b"}} {
		if !none.Matches(tags) {
			t.Errorf("empty set does not match %q, want every repository kept", tags)
		}
	}
}

// TestTagSetMatchesIsUnion checks that a repository matching any tag matches.
func TestTagSetMatchesIsUnion(t *testing.T) {
	t.Parallel()

	set := NewTagSet([]string{"demo,backend"})
	tests := []struct {
		name string
		tags []Tag
		want bool
	}{
		{"carries one", []Tag{"demo"}, true},
		{"carries the other", []Tag{"backend"}, true},
		{"carries both", []Tag{"demo", "backend"}, true},
		{"carries one among others", []Tag{"web", "backend", "infra"}, true},
		{"carries none", []Tag{"web"}, false},
		{"carries nothing at all", nil, false},
		{"a prefix is not a match", []Tag{"dem"}, false},
		{"a superstring is not a match", []Tag{"demos"}, false},
		{"case is folded", []Tag{"DEMO"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := set.Matches(tt.tags); got != tt.want {
				t.Errorf("NewTagSet(demo,backend).Matches(%q) = %v, want %v",
					tt.tags, got, tt.want)
			}
		})
	}
}

// TestResolveTags checks that tags are inherited at any depth.
func TestResolveTags(t *testing.T) {
	t.Parallel()

	project := Project{
		Name:  "root",
		Tags:  []Tag{"work"},
		Repos: []Repository{{Name: "api", Tags: []Tag{"demo"}}, {Name: "web"}},
		SubProjects: []Project{{
			Name:  "infra",
			Tags:  []Tag{"ops"},
			Repos: []Repository{{Name: "db", Tags: []Tag{"Demo"}}},
			SubProjects: []Project{{
				Name:  "deep",
				Repos: []Repository{{Name: "cache"}},
			}},
		}},
	}
	project.ResolveTags(nil)

	want := map[string][]Tag{
		"api":   {"demo", "work"},
		"web":   {"work"},
		"db":    {"demo", "ops", "work"},
		"cache": {"ops", "work"},
	}
	for name, wantTags := range want {
		repo, found := project.GetRepo(name, "")
		if !found {
			t.Fatalf("repository %q missing from the tree", name)
		}
		if !reflect.DeepEqual(repo.Tags, wantTags) {
			t.Errorf("%s tags = %q, want %q", name, repo.Tags, wantTags)
		}
	}
}

// TestFilterTags checks filtering across sub-projects and the match count.
func TestFilterTags(t *testing.T) {
	t.Parallel()

	newTree := func() Project {
		p := Project{
			Name:  "root",
			Repos: []Repository{{Name: "api", Tags: []Tag{"demo"}}, {Name: "web"}},
			SubProjects: []Project{{
				Name:  "infra",
				Repos: []Repository{{Name: "db", Tags: []Tag{"demo"}}, {Name: "dns"}},
			}},
		}
		p.ResolveTags(nil)
		return p
	}

	t.Run("keeps the tagged ones at every depth", func(t *testing.T) {
		t.Parallel()

		p := newTree()
		if kept := p.FilterTags(NewTagSet([]string{"demo"})); kept != 2 {
			t.Errorf("FilterTags kept = %d, want 2", kept)
		}
		if names := p.ListReposWithNamespace(); !reflect.DeepEqual(names, []string{"api", "infra/db"}) {
			t.Errorf("repositories = %q, want api and infra/db", names)
		}
	})

	t.Run("an empty set keeps everything", func(t *testing.T) {
		t.Parallel()

		p := newTree()
		if kept := p.FilterTags(TagSet{}); kept != 4 {
			t.Errorf("FilterTags(empty) kept = %d, want every repository (4)", kept)
		}
	})

	t.Run("a tag nothing carries keeps nothing", func(t *testing.T) {
		t.Parallel()

		p := newTree()
		if kept := p.FilterTags(NewTagSet([]string{"ghost"})); kept != 0 {
			t.Errorf("FilterTags(ghost) kept = %d, want 0", kept)
		}
		// Empty sub-projects are kept.
		if len(p.SubProjects) != 1 {
			t.Errorf("sub-projects = %d, want the emptied node kept", len(p.SubProjects))
		}
	})
}

// TestFilterTagsAfterExclude checks that excluded repositories stay excluded.
func TestFilterTagsAfterExclude(t *testing.T) {
	t.Parallel()

	p := Project{
		Name:    "root",
		Exclude: []string{"legacy"},
		Repos: []Repository{
			{Name: "api", Tags: []Tag{"demo"}},
			{Name: "legacy", Tags: []Tag{"demo"}},
		},
	}
	p.ResolveTags(nil)
	p.Filter()
	p.FilterTags(NewTagSet([]string{"demo"}))

	if names := p.ListReposWithNamespace(); !reflect.DeepEqual(names, []string{"api"}) {
		t.Errorf("repositories = %q, want the excluded one to stay excluded", names)
	}
}
