package json

import "time"

// StatusReport is one repository's working-tree data: either a Status, when
// git answered, or a StatusError, when the probe failed.
//
// The two are separate types rather than one struct with an error field
// because a failed probe measured nothing, and `"staged": 0` beside an error
// is indistinguishable from a clean work tree. Making them distinct types
// means a caller cannot accidentally report both.
type StatusReport interface{ isStatusReport() }

// Status is what git reported about a repository's working tree. Its counts
// are always emitted: a zero is a measurement, and the absence of the whole
// object — not of a field — is what says nothing was measured.
type Status struct {
	Branch    string `json:"branch"`
	Staged    int    `json:"staged"`
	Unstaged  int    `json:"unstaged"`
	Untracked int    `json:"untracked"`

	Ahead  int `json:"ahead"`
	Behind int `json:"behind"`
	// Compared reports whether Ahead and Behind are the result of an actual
	// comparison — against the Upstream, or against a same-named branch on one
	// of the Remotes when no Upstream resolves. False means both are zero for
	// want of a reference, not because the branch is level.
	//
	// Deliberately not named for the Upstream: the fallback compares against a
	// Remote branch that no local branch tracks.
	Compared bool `json:"compared"`

	// Upstream is absent when none is configured, so the three states are
	// structural: absent means none, present and tracked means healthy,
	// present and untracked is a Gone Upstream.
	Upstream *Upstream `json:"upstream,omitempty"`

	// Version is `git describe`, absent when no tag is reachable.
	Version string `json:"version,omitempty"`
	// Head is the uncommitted line diff against HEAD, measured only under
	// --stat. Absent means unmeasured — never zero.
	Head *Head `json:"head,omitempty"`
	// Commit is the last commit, absent when HEAD could not be read.
	Commit *Commit `json:"commit,omitempty"`
}

func (*Status) isStatusReport() {}

// StatusError is the status of a repository whose probe failed: the reason,
// and nothing else.
type StatusError struct {
	Error string `json:"error"`
}

func (*StatusError) isStatusReport() {}

// FailedStatus reports a probe that could not command.
func FailedStatus(err error) *StatusError {
	return &StatusError{Error: err.Error()}
}

// Upstream is the remote branch a local branch tracks.
type Upstream struct {
	// Name is the Upstream's short name ("origin/feat-b"). A branch tracking
	// another local branch carries no remote prefix.
	Name string `json:"name"`
	// Tracked reports whether a ref still resolves behind that name. False is
	// the Gone Upstream: configured, but merged and cleaned up.
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
