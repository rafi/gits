package domain

import "strings"

// Project represents a single project that can have many child projects,
// while each project can have many repositories.
type Project struct {
	Source      *ProviderSource `json:"source,omitempty"`
	Clone       *bool           `json:"clone,omitempty"`
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Path        string          `json:"path"`
	Desc        string          `json:"desc,omitempty"`
	AbsPath     string          `json:"-"`
	Repos       []Repository    `json:"repos,omitempty"`
	SubProjects []Project       `json:"subprojects,omitempty"`
}

// ProjectList is a list of projects with keys.
type ProjectListKeyed map[string]Project

func (p Project) GetRepo(name, prefix string) (Repository, bool) {
	for _, repo := range p.Repos {
		if prefix+repo.GetName() == name {
			return repo, true
		}
	}
	for _, subProj := range p.SubProjects {
		subPrefix := prefix + subProj.Name + "/"
		if r, found := subProj.GetRepo(name, subPrefix); found {
			return r, true
		}
	}
	return Repository{}, false
}

func (p Project) GetSubProject(name, prefix string) (Project, bool) {
	name = strings.Trim(name, "/")

	if name+"/" == prefix {
		return p, true
	}

	for _, subProj := range p.SubProjects {
		subPrefix := prefix + subProj.Name + "/"
		proj, found := subProj.GetSubProject(name, subPrefix)
		if found {
			return proj, true
		}
	}
	return Project{}, false
}

func (p Project) GetAllRepos(prefix ...string) []Repository {
	var repos []Repository
	if len(prefix) == 0 {
		prefix = []string{""}
	}
	repos = append(repos, p.Repos...)
	for _, subProj := range p.SubProjects {
		subPrefix := prefix[0] + subProj.Name + "/"
		repos = append(repos, subProj.GetAllRepos(subPrefix)...)
	}
	return repos
}

func (p Project) ListReposWithNamespace(prefix ...string) []string {
	var names []string
	if len(prefix) == 0 {
		prefix = []string{""}
	}
	for _, repo := range p.Repos {
		names = append(names, prefix[0]+repo.GetName())
	}
	for _, subProj := range p.SubProjects {
		subPrefix := prefix[0] + subProj.Name + "/"
		names = append(names, subProj.ListReposWithNamespace(subPrefix)...)
	}
	return names
}
