// Package wire is the JSON document gits emits: the envelope `list`, `status`
// and the line commands share, and the conversion into it from the domain
// tree.
//
// It is the business layer's serialization, not a CLI rendering choice — an
// HTTP handler serving a project's status returns this same document. Nothing
// here knows about terminals, themes or writers beyond an [io.Writer].
//
// The types mirror the domain deliberately rather than marshaling it
// directly: that would make every domain field decision a wire decision, and
// it would put Repo State into the cache file, which marshals the domain
// types. Identity fields are projected from domain.RepoIdentity rather than
// restated, so the two cannot drift.
//
// Every command emits one shape so a consumer need not know which produced
// its input. They differ in one way: `status` nests a working-tree object
// under each repository it probed, a line command nests a command object, and
// `list` nests neither — presence, not a zero value, says what was run.
package wire

import (
	"github.com/rafi/gits/domain"
)

// Envelope is the whole document: projects keyed by name. encoding/json emits
// map keys sorted, so output is stable across runs.
type Envelope map[string]Project

// Project is one node of the project tree.
type Project struct {
	domain.ProjectIdentity

	Repos       []Repository `json:"repos,omitempty"`
	SubProjects []Project    `json:"subprojects,omitempty"`
	Include     []string     `json:"include,omitempty"`
	Exclude     []string     `json:"exclude,omitempty"`
}

// Repository is one repository's identity and Repo State, plus whichever of
// the two per-command objects applies: `status`'s working tree, or a line
// command's outcome.
type Repository struct {
	domain.RepoIdentity

	// State is one of the five Repo State strings — wire vocabulary, never
	// rendered as display text here.
	State domain.RepoState `json:"state"`
	// Reason explains the error state, and is populated for no other.
	Reason string `json:"reason,omitempty"`

	// Status is present only when git was consulted for this repository.
	// That presence is the statement, not the values inside it. It holds a
	// *Status when the probe answered and a *StatusError when it failed.
	Status StatusReport `json:"status,omitempty"`
	// Command is what a line command made of this repository, present only
	// when one ran for it.
	Command *Command `json:"command,omitempty"`
}

// FromProjects converts a keyed project list into the envelope, with no
// working-tree data attached — the shape `list -o json` emits.
func FromProjects(projects domain.ProjectListKeyed) Envelope {
	env := make(Envelope, len(projects))
	for name, project := range projects {
		env[name] = FromProject(project)
	}
	return env
}

// FromProject converts one project subtree, repositories and sub-projects
// included.
func FromProject(project domain.Project) Project {
	out := NewProject(project)
	for _, repo := range project.Repos {
		out.Repos = append(out.Repos, NewRepository(repo))
	}
	for _, sub := range project.SubProjects {
		out.SubProjects = append(out.SubProjects, FromProject(sub))
	}
	return out
}

// NewProject converts a single project node without descending into it, for
// callers that fill the repositories themselves.
func NewProject(project domain.Project) Project {
	return Project{
		ProjectIdentity: project.Identity(),
		Include:         project.Include,
		Exclude:         project.Exclude,
	}
}

// NewRepository converts one repository's identity and state. Reason travels
// only with the error state it explains.
func NewRepository(repo domain.Repository) Repository {
	out := Repository{
		RepoIdentity: repo.Identity(),
		State:        repo.State,
	}
	if repo.State == domain.RepoStateError {
		out.Reason = repo.Reason
	}
	return out
}
