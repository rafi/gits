package domain

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
