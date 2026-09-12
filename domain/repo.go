package domain

import (
	"path/filepath"
	"slices"
	"strings"
)

// Repository represents a single repository from filesystem or git provider.
type Repository struct {
	ID        string `json:"id,omitempty"`
	Name      string `json:"name,omitempty"`
	Namespace string `json:"namespace,omitempty"`
	Src       string `json:"src,omitempty"`
	Dir       string `json:"dir,omitempty"`
	URL       string `json:"url,omitempty"`
	Desc      string `json:"desc,omitempty"`

	Type    string    `json:"-"`
	AbsPath string    `json:"-"`
	State   RepoState `json:"-"`
	Reason  string    `json:"-"`
}

// RepoState represents what is known about a repository's local presence.
//
// These strings are wire vocabulary: they appear verbatim in JSON output and
// are a public contract. Display layers map them to icons and labels, never
// the reverse.
type RepoState string

const (
	// RepoStateUnknown means the repository has not been classified yet.
	RepoStateUnknown RepoState = "unknown"
	// RepoStateError means the repository's configuration is defective, or a
	// path exists but is not a readable git repository. Carries a Reason.
	RepoStateError RepoState = "error"
	// RepoStateRemoteOnly means the repository is provider-backed and the
	// configuration gives it no local home. There is nothing to clone into.
	RepoStateRemoteOnly RepoState = "remote-only"
	// RepoStateNotCloned means a local path is known and nothing is there.
	// This is the state `gits clone` acts on.
	RepoStateNotCloned RepoState = "not-cloned"
	// RepoStateOK means a local clone exists and is readable.
	RepoStateOK RepoState = "ok"
)

// GetName returns the repository's display name: its configured name, else
// the basename of its Repo Dir or Repo Src, else a placeholder.
func (r Repository) GetName() string {
	switch {
	case r.Name != "":
		return r.Name
	case r.Dir != "":
		return filepath.Base(r.Dir)
	case r.Src != "":
		return filepath.Base(r.Src)
	default:
		return "<unnamed>"
	}
}

// GetNameWithNamespace returns GetName prefixed with the repository's
// namespace, when it has one.
func (r Repository) GetNameWithNamespace() string {
	title := r.GetName()
	if r.Namespace != "" {
		title = strings.Join([]string{r.Namespace, title}, "/")
	}
	return title
}

// GetSource returns the repository's Repo Src, falling back to its Reason
// when it has none — an `error` repository has a reason and no source.
func (r Repository) GetSource() string {
	switch {
	case r.Src != "":
		return r.Src
	default:
		return r.Reason
	}
}

// ContainedIn reports whether any of paths names this repository — by name,
// by namespace, or by namespaced name.
func (r Repository) ContainedIn(paths []string) bool {
	keys := []string{
		r.GetName(),
		r.Namespace,
		r.GetNameWithNamespace(),
	}
	return slices.ContainsFunc(paths, func(exclude string) bool {
		return slices.Index(keys, exclude) > -1
	})
}
