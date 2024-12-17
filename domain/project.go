package domain

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mitchellh/go-homedir"
)

// Project represents a single project that can have many child projects,
// while each project can have many repositories.
type Project struct {
	Source      *ProviderSource `json:"source,omitempty"`
	Clone       *bool           `json:"clone,omitempty"`
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Path        string          `json:"path"`
	Desc        string          `json:"desc,omitempty"`
	Hash        string          `json:"-"`
	AbsPath     string          `json:"-"`
	Repos       []Repository    `json:"repos,omitempty"`
	SubProjects []Project       `json:"subprojects,omitempty"`
	Include     []string        `json:"include,omitempty"`
	Exclude     []string        `json:"exclude,omitempty"`
}

// ProjectList is a list of projects with keys.
type ProjectListKeyed map[string]Project

// GetRepo returns a repository by name and initial prefix.
func (p *Project) GetRepo(name, prefix string) (Repository, bool) {
	for _, repo := range p.Repos {
		switch name {
		case prefix + repo.GetName():
			fallthrough
		case repo.GetNameWithNamespace():
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

// GetSubProject returns a sub-project by name and initial prefix.
func (p *Project) GetSubProject(name, prefix string) (Project, bool) {
	name = strings.Trim(name, "/")

	if name+"/" == prefix {
		return *p, true
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

// GetAllRepos returns a list of all repositories in the project.
func (p *Project) GetAllRepos(prefix ...string) []Repository {
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

// ListReposWithNamespace returns a list of repository names with namespace.
func (p *Project) ListReposWithNamespace(prefix ...string) []string {
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

	// Sort sub-projects and repositories alphabetically.
	sort.Strings(names)
	return names
}

// GetRepoAbsPath returns an absolute path of one of its repositories.
func (p *Project) GetRepoAbsPath(repo Repository) (string, error) {
	path := filepath.Clean(p.AbsPath)
	if len(repo.Dir) == 0 {
		lastSlash := strings.LastIndex(repo.Src, "/")
		if lastSlash == -1 {
			return "", fmt.Errorf("unable to get repo path %s", repo.Src)
		}
		name := repo.Src[lastSlash+1:]
		name = strings.TrimSuffix(name, filepath.Ext(name))
		return filepath.Join(path, name), nil
	}
	expanded, err := homedir.Expand(repo.Dir)
	if err != nil {
		return "", fmt.Errorf("unable to expand path: %w", err)
	}
	if string(expanded[0]) == "/" {
		path = filepath.Clean(expanded)
	} else {
		path = filepath.Join(path, expanded)
	}
	return path, nil
}

// Filter filters the project repositories by user include/exclude filters.
func (p *Project) Filter() {
	repos := []Repository{}
	for _, repo := range p.Repos {
		// Disregard excluded repositories.
		if repo.ContainedIn(p.Exclude) {
			continue
		}
		// If include list is provided, it is explicit.
		if len(p.Include) > 0 && !repo.ContainedIn(p.Include) {
			continue
		}
		repos = append(repos, repo)
	}
	p.Repos = repos

	// Recurse into subprojects.
	for _, subProject := range p.SubProjects {
		subProject.Filter()
	}
}

func (p *Project) CalculateHash() error {
	data, err := json.Marshal(p)
	if err != nil {
		return err
	}
	hash := md5.Sum(data)
	p.Hash = hex.EncodeToString(hash[:])
	return nil
}
