package domain

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/mitchellh/go-homedir"
)

// sampleProject builds a nested project tree:
//
//	root
//	  repos: api
//	  sub: backend
//	    repos: db (namespace infra)
//	    sub: service
//	      repos: cache
func sampleProject() Project {
	return Project{
		Name:  "root",
		Repos: []Repository{{Name: "api"}},
		SubProjects: []Project{{
			Name:  "backend",
			Repos: []Repository{{Name: "db", Namespace: "infra"}},
			SubProjects: []Project{{
				Name:  "service",
				Repos: []Repository{{Name: "cache"}},
			}},
		}},
	}
}

func TestProjectGetRepo(t *testing.T) {
	t.Parallel()

	p := sampleProject()
	tests := []struct {
		name      string
		query     string
		wantName  string
		wantFound bool
	}{
		{"top-level by prefixed name", "api", "api", true},
		{"nested by prefixed name", "backend/db", "db", true},
		{"deeply nested by prefixed name", "backend/service/cache", "cache", true},
		{"by name with namespace", "infra/db", "db", true},
		{"not found", "missing", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			repo, found := p.GetRepo(tt.query, "")
			if found != tt.wantFound {
				t.Fatalf("GetRepo(%q) found = %v, want %v", tt.query, found, tt.wantFound)
			}
			if found && repo.GetName() != tt.wantName {
				t.Errorf("GetRepo(%q) = %q, want %q", tt.query, repo.GetName(), tt.wantName)
			}
		})
	}
}

func TestProjectGetSubProject(t *testing.T) {
	t.Parallel()

	p := sampleProject()
	tests := []struct {
		name      string
		query     string
		wantName  string
		wantFound bool
	}{
		{"top-level sub", "backend", "backend", true},
		{"nested sub", "backend/service", "service", true},
		{"trims surrounding slashes", "/backend/", "backend", true},
		{"not found", "nope", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			sub, found := p.GetSubProject(tt.query, "")
			if found != tt.wantFound {
				t.Fatalf("GetSubProject(%q) found = %v, want %v", tt.query, found, tt.wantFound)
			}
			if found && sub.Name != tt.wantName {
				t.Errorf("GetSubProject(%q) = %q, want %q", tt.query, sub.Name, tt.wantName)
			}
		})
	}
}

func TestProjectListReposWithNamespace(t *testing.T) {
	t.Parallel()

	p := Project{
		Name:  "root",
		Repos: []Repository{{Name: "zebra"}, {Name: "apple"}},
		SubProjects: []Project{{
			Name:  "sub",
			Repos: []Repository{{Name: "mango"}},
		}},
	}
	want := []string{"apple", "sub/mango", "zebra"}
	got := p.ListReposWithNamespace()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ListReposWithNamespace() = %v, want %v", got, want)
	}
}

func TestProjectGetRepoAbsPath(t *testing.T) {
	t.Parallel()

	tildeExpanded, err := homedir.Expand("~/foo")
	if err != nil {
		t.Fatalf("homedir.Expand: %v", err)
	}

	tests := []struct {
		name    string
		project Project
		repo    Repository
		want    string
		wantErr bool
	}{
		{
			name:    "name derived from src when dir empty",
			project: Project{AbsPath: "/home/proj"},
			repo:    Repository{Src: "git@host:owner/myrepo.git"},
			want:    "/home/proj/myrepo",
		},
		{
			name:    "src without slash errors",
			project: Project{AbsPath: "/home/proj"},
			repo:    Repository{Src: "noslash"},
			wantErr: true,
		},
		{
			// Only a ".git" suffix is stripped — a repo named next.js must
			// not lose its ".js".
			name:    "dotted repo name keeps its extension",
			project: Project{AbsPath: "/home/proj"},
			repo:    Repository{Src: "https://host/owner/next.js"},
			want:    "/home/proj/next.js",
		},
		{
			name:    "absolute dir used directly",
			project: Project{AbsPath: "/home/proj"},
			repo:    Repository{Dir: "/abs/path"},
			want:    "/abs/path",
		},
		{
			name:    "relative dir joined to abspath",
			project: Project{AbsPath: "/home/proj"},
			repo:    Repository{Dir: "sub/dir"},
			want:    "/home/proj/sub/dir",
		},
		{
			name:    "tilde expanded",
			project: Project{AbsPath: "/home/proj"},
			repo:    Repository{Dir: "~/foo"},
			want:    filepath.Clean(tildeExpanded),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := tt.project.GetRepoAbsPath(tt.repo)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("GetRepoAbsPath() = %q, want error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("GetRepoAbsPath() unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("GetRepoAbsPath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestProjectFilter(t *testing.T) {
	t.Parallel()

	p := Project{
		Include: []string{"keep", "both"},
		Exclude: []string{"drop", "both"},
		Repos: []Repository{
			{Name: "keep"}, {Name: "drop"}, {Name: "other"}, {Name: "both"},
		},
		SubProjects: []Project{{
			Name:    "sub",
			Exclude: []string{"x"},
			Repos:   []Repository{{Name: "a"}, {Name: "x"}},
		}},
	}
	p.Filter()

	gotRoot := repoNames(p.Repos)
	if !reflect.DeepEqual(gotRoot, []string{"keep"}) {
		t.Errorf("root repos = %v, want [keep] (exclude beats include)", gotRoot)
	}
	gotSub := repoNames(p.SubProjects[0].Repos)
	if !reflect.DeepEqual(gotSub, []string{"a"}) {
		t.Errorf("sub repos = %v, want [a] (recursion + exclude)", gotSub)
	}
}

func repoNames(repos []Repository) []string {
	out := make([]string, 0, len(repos))
	for _, r := range repos {
		out = append(out, r.GetName())
	}
	return out
}

func TestProjectCalculateHash(t *testing.T) {
	t.Parallel()

	p := sampleProject()
	if err := p.CalculateHash(); err != nil {
		t.Fatalf("CalculateHash: %v", err)
	}
	first := p.Hash
	if first == "" {
		t.Fatal("hash is empty")
	}

	// Stable: recomputing yields the same hash.
	if err := p.CalculateHash(); err != nil {
		t.Fatalf("CalculateHash: %v", err)
	}
	if p.Hash != first {
		t.Errorf("hash not stable: %q then %q", first, p.Hash)
	}

	// Sensitive: adding a repo changes the hash.
	p.Repos = append(p.Repos, Repository{Name: "new"})
	if err := p.CalculateHash(); err != nil {
		t.Fatalf("CalculateHash: %v", err)
	}
	if p.Hash == first {
		t.Error("hash did not change after adding a repo")
	}
}
