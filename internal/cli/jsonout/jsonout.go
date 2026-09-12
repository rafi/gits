// Package jsonout holds the JSON envelope `gits list -o json`,
// `gits status -o json` and the line Bulk Commands' `-o json` share, and the
// conversion into it from the domain tree.
//
// The types here are the wire contract, deliberately separate from the domain
// structs they mirror. Marshaling `domain.Project` directly — as `list` used
// to — made every field decision in the domain a wire decision by accident,
// and left `status` nowhere to attach its nested working-tree object without
// diverging from `list`. The separation also keeps `state` out of the cache
// file and out of `domain.Project.CalculateHash`, both of which marshal the
// domain type.
//
// Every command emits one shape rather than its own so a consumer need not
// know which produced its input. They differ in exactly one way: `status`
// nests a working-tree object under each repository it probed, a line Bulk
// Command (`pull`, `fetch`, `push`, `clone`, `exec`) nests an Outcome under
// its own name for each repository it ran on, and `list` nests nothing — so
// the key's presence, rather than a zero count, is what says the command was
// run, and the key's name says which.
package jsonout

import (
	"bytes"
	"encoding/json"
	"errors"
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

	// Outcome is what a line Bulk Command made of this repository, present
	// only when the command ran for it. It is nested under Command — the
	// command's own name, `pull` for `gits pull` — so the key says which
	// command ran, as `status` does. The key is not a struct tag's to give,
	// so MarshalJSON adds it; neither field reaches the document by itself.
	Command string   `json:"-"`
	Outcome *Outcome `json:"-"`
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

	// Upstream is the branch's Upstream, absent when none is configured — so
	// the three states are structural: absent means none, present and tracked
	// means healthy, present and untracked is a Gone Upstream. Orthogonal to
	// Compared, which is about whether a comparison happened at all.
	Upstream *Upstream `json:"upstream,omitempty"`

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

// Upstream is the remote branch a local branch tracks.
type Upstream struct {
	// Name is the Upstream's short name ("origin/feat-b"). A branch tracking
	// another local branch carries no remote prefix.
	Name string `json:"name"`
	// Tracked reports whether a ref still resolves behind that name. False is
	// the Gone Upstream: configured, but merged and cleaned up on the Remote.
	Tracked bool `json:"tracked"`
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

// Outcome is what one of the line Bulk Commands made of a repository. Exactly
// one field is set, and MarshalJSON emits only that one: a command either
// produced output, passed the repository over for a documented reason, or
// failed on it, and `"output": ""` beside an error would read as a command
// that ran and said nothing.
type Outcome struct {
	// Output is what the command printed for the repository — git's own
	// report, or the child's combined output under `exec` — with no
	// terminal styling. Present, even when empty, whenever the command
	// succeeded.
	Output string `json:"output"`
	// Skipped is the reason the command passed the repository over: a
	// documented pass-over, such as a branch with no Upstream, which the
	// table shows on the line and which does not fail the run. It is not an
	// error, and is not reported as one.
	Skipped string `json:"skipped,omitempty"`
	// Error is why the command failed on the repository.
	Error string `json:"error,omitempty"`
}

// MarshalJSON emits the one field that describes the outcome — see Outcome.
func (o Outcome) MarshalJSON() ([]byte, error) {
	switch {
	case o.Error != "":
		return json.Marshal(struct {
			Error string `json:"error"`
		}{Error: o.Error})
	case o.Skipped != "":
		return json.Marshal(struct {
			Skipped string `json:"skipped"`
		}{Skipped: o.Skipped})
	default:
		return json.Marshal(struct {
			Output string `json:"output"`
		}{Output: o.Output})
	}
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

// errOutcomeWithoutCommand is returned when an Outcome has no key to nest
// under: a renderer that forgot to name its command, caught at marshal time
// rather than by emitting a document with a nameless object.
var errOutcomeWithoutCommand = errors.New("jsonout: an Outcome needs the command's name to nest under")

// MarshalJSON is the default struct encoding, plus the Outcome under the
// Command's name when there is one. The tagged fields are encoded first, on
// their own, so a repository without an Outcome — every one `list` and
// `status` emit — is byte-for-byte what the plain encoding produces.
func (r Repository) MarshalJSON() ([]byte, error) {
	// The alias sheds this method, so the default struct encoding applies.
	type alias Repository
	raw, err := json.Marshal(alias(r))
	if err != nil || r.Outcome == nil {
		return raw, err
	}
	if r.Command == "" {
		return nil, errOutcomeWithoutCommand
	}
	key, err := json.Marshal(r.Command)
	if err != nil {
		return nil, err
	}
	value, err := json.Marshal(r.Outcome)
	if err != nil {
		return nil, err
	}
	// raw is an object holding at least `state`, so it ends in `}` and a
	// comma before the added member is always right.
	var buf bytes.Buffer
	buf.Write(raw[:len(raw)-1])
	buf.WriteByte(',')
	buf.Write(key)
	buf.WriteByte(':')
	buf.Write(value)
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// Write marshals env to w as one newline-terminated line.
func Write(w io.Writer, env Envelope) error {
	return WriteValue(w, env)
}

// WriteValue marshals any document to w as one newline-terminated line — the
// shape that makes Result Output pipeable into `jq` without a reader having to
// know how many lines to expect.
//
// It exists for `doctor`, whose report is a list of findings rather than a
// project tree and so cannot use Envelope. What the two share is this
// convention, not the schema, and that is exactly what is factored here: a
// command emitting a different document still emits it the same way.
func WriteValue(w io.Writer, doc any) error {
	raw, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	_, err = w.Write(append(raw, '\n'))
	return err
}
