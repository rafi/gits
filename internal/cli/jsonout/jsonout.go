// Package jsonout holds the JSON envelope `gits list -o json` and
// `gits status -o json` share, and the conversion into it from the domain
// tree.
//
// The types here are the wire contract, deliberately separate from the domain
// structs they mirror. Marshalling `domain.Project` directly — as `list` used
// to — made every field decision in the domain a wire decision by accident,
// and left `status` nowhere to attach its nested working-tree object without
// diverging from `list`. The separation also keeps `state` out of the cache
// file and out of `domain.Project.CalculateHash`, both of which marshal the
// domain type.
//
// The two commands emit one shape rather than two so a consumer need not know
// which produced its input. They differ in exactly one way: `status` nests a
// working-tree object under each repository it probed, and `list` never does
// — so the key's presence, rather than a zero count, is what says git was
// consulted.
package jsonout

import (
	"encoding/json"
	"io"
	"time"

	"github.com/rafi/gits/domain"
)

// Envelope is the whole document: projects keyed by name. encoding/json emits
// map keys sorted, so the output is stable across runs.
type Envelope map[string]Project

// Project is one node of the project tree.
type Project struct {
	Source      *domain.ProviderSource `json:"source,omitempty"`
	Clone       *bool                  `json:"clone,omitempty"`
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Path        string                 `json:"path"`
	Desc        string                 `json:"desc,omitempty"`
	Repos       []Repository           `json:"repos,omitempty"`
	SubProjects []Project              `json:"subprojects,omitempty"`
	Include     []string               `json:"include,omitempty"`
	Exclude     []string               `json:"exclude,omitempty"`
}

// Repository is one repository's identity and Repo State, plus — from
// `status` only — the working-tree data git was asked for.
type Repository struct {
	ID        string `json:"id,omitempty"`
	Name      string `json:"name,omitempty"`
	Namespace string `json:"namespace,omitempty"`
	Src       string `json:"src,omitempty"`
	Dir       string `json:"dir,omitempty"`
	URL       string `json:"url,omitempty"`
	Desc      string `json:"desc,omitempty"`

	// State is one of the five Repo State strings. They are wire vocabulary
	// and are never rendered as display text here.
	State domain.RepoState `json:"state"`
	// Reason explains the error state, and is populated for no other.
	Reason string `json:"reason,omitempty"`

	// Status is present only when git was consulted for this repository —
	// that presence is the statement, not the values inside it.
	Status *Status `json:"status,omitempty"`
}

// Status is one repository's working-tree data, nested so that `list`'s
// silence about it is structural rather than a row of zeroes.
type Status struct {
	Branch    string `json:"branch"`
	Staged    int    `json:"staged"`
	Unstaged  int    `json:"unstaged"`
	Untracked int    `json:"untracked"`

	Ahead  int `json:"ahead"`
	Behind int `json:"behind"`
	// Compared reports whether Ahead and Behind are the result of an actual
	// comparison. True when the branch has an Upstream, or — when it has
	// none — when a branch of the same name was found on one of the
	// repository's Remotes to compare against instead. False means nothing
	// could be compared and both counts are zero for want of a reference,
	// not because the branch is level.
	//
	// Deliberately not named for the Upstream: the fallback compares against
	// a Remote branch that no local branch tracks, and conflating the two is
	// the distinction CONTEXT.md calls load-bearing.
	Compared bool `json:"compared"`

	// Version is `git describe`, absent when the repository has no tag to
	// describe against.
	Version string `json:"version,omitempty"`
	// Head is the uncommitted line diff against HEAD, measured only under
	// --stat. Absent means unmeasured — never zero.
	Head *Head `json:"head,omitempty"`
	// Commit is the last commit, absent when HEAD could not be read.
	Commit *Commit `json:"commit,omitempty"`

	// Error is the reason the probe failed. When it is set, nothing else was
	// measured — see MarshalJSON.
	Error string `json:"error,omitempty"`
}

// Head is the uncommitted line diff against HEAD.
type Head struct {
	Added   int `json:"added"`
	Deleted int `json:"deleted"`
}

// Commit is a repository's last commit.
type Commit struct {
	Hash    string    `json:"hash"`
	Subject string    `json:"subject"`
	Time    time.Time `json:"time"`
}

// MarshalJSON emits nothing but the error when the probe failed. A failed
// probe measured no counts, and `"staged": 0` beside an error is
// indistinguishable from a clean work tree — the same reason the whole object
// is nested rather than flattened into the repository.
func (s Status) MarshalJSON() ([]byte, error) {
	if s.Error != "" {
		return json.Marshal(struct {
			Error string `json:"error"`
		}{Error: s.Error})
	}
	// The alias sheds this method, so the default struct encoding applies.
	type alias Status
	return json.Marshal(alias(s))
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
		Source:  project.Source,
		Clone:   project.Clone,
		ID:      project.ID,
		Name:    project.Name,
		Path:    project.Path,
		Desc:    project.Desc,
		Include: project.Include,
		Exclude: project.Exclude,
	}
}

// NewRepository converts one repository's identity and state. Reason travels
// only with the error state it explains.
func NewRepository(repo domain.Repository) Repository {
	out := Repository{
		ID:        repo.ID,
		Name:      repo.Name,
		Namespace: repo.Namespace,
		Src:       repo.Src,
		Dir:       repo.Dir,
		URL:       repo.URL,
		Desc:      repo.Desc,
		State:     repo.State,
	}
	if repo.State == domain.RepoStateError {
		out.Reason = repo.Reason
	}
	return out
}

// Write marshals env to w as one newline-terminated line.
func Write(w io.Writer, env Envelope) error {
	raw, err := json.Marshal(env)
	if err != nil {
		return err
	}
	_, err = w.Write(append(raw, '\n'))
	return err
}
