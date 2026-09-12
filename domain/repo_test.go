package domain

import "testing"

func TestRepositoryGetName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		repo Repository
		want string
	}{
		{"name wins", Repository{Name: "foo", Dir: "/x/dir", Src: "git@h:o/src.git"}, "foo"},
		{"dir base when no name", Repository{Dir: "/x/y/dir"}, "dir"},
		{"src base when no name or dir", Repository{Src: "git@host:owner/src"}, "src"},
		{"unnamed fallback", Repository{}, "<unnamed>"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.repo.GetName(); got != tt.want {
				t.Errorf("GetName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRepositoryGetNameWithNamespace(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		repo Repository
		want string
	}{
		{"with namespace", Repository{Name: "repo", Namespace: "team"}, "team/repo"},
		{"without namespace", Repository{Name: "repo"}, "repo"},
		{"namespace with fallback name", Repository{Namespace: "team"}, "team/<unnamed>"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.repo.GetNameWithNamespace(); got != tt.want {
				t.Errorf("GetNameWithNamespace() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRepositoryGetSource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		repo Repository
		want string
	}{
		{"src wins", Repository{Src: "git@host:o/r", Reason: "because"}, "git@host:o/r"},
		{"reason fallback when no src", Repository{Reason: "because"}, "because"},
		{"empty when neither", Repository{}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.repo.GetSource(); got != tt.want {
				t.Errorf("GetSource() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRepositoryContainedIn(t *testing.T) {
	t.Parallel()

	repo := Repository{Name: "repo", Namespace: "team"}
	tests := []struct {
		name  string
		paths []string
		want  bool
	}{
		{"matches name", []string{"repo"}, true},
		{"matches namespace", []string{"team"}, true},
		{"matches name-with-namespace", []string{"team/repo"}, true},
		{"no false positive", []string{"other", "team/other"}, false},
		{"empty paths", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := repo.ContainedIn(tt.paths); got != tt.want {
				t.Errorf("ContainedIn(%v) = %v, want %v", tt.paths, got, tt.want)
			}
		})
	}
}

// TestRepositoryKeyIdentifiesWithinATree: the key separates repositories that
// share a name but not a path, and those with no resolved path at all.
func TestRepositoryKeyIdentifiesWithinATree(t *testing.T) {
	t.Parallel()

	a := Repository{Name: "api", AbsPath: "/code/acme/api"}
	b := Repository{Name: "api", AbsPath: "/code/other/api"}
	unresolved1 := Repository{Dir: "api"}
	unresolved2 := Repository{Dir: "web"}

	if a.Key() != (Repository{Name: "api", AbsPath: "/code/acme/api"}).Key() {
		t.Errorf("Key() differs for equal repositories")
	}
	if a.Key() == b.Key() {
		t.Errorf("Key() = %q for both paths, want the path to separate them", a.Key())
	}
	if unresolved1.Key() == unresolved2.Key() {
		t.Errorf("Key() = %q for both, want the name to separate pathless repositories",
			unresolved1.Key())
	}
}
