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

// RepoState represents the state of a repository.
type RepoState string

var (
	RepoStateUnknown RepoState = "Unknown"
	RepoStateError   RepoState = "Error"
	RepoStateRemote  RepoState = "Remote"
	RepoStateNoLocal RepoState = "N/A"
	RepoStateOK      RepoState = "OK"
)

func (r Repository) GetName() string {
	title := ""
	switch {
	case r.Name != "":
		title = r.Name
	case r.Dir != "":
		title = filepath.Base(r.Dir)
	case r.Src != "":
		title = filepath.Base(r.Src)
	default:
		title = "<unnamed>"
	}
	return title
}

func (r Repository) GetNameWithNamespace() string {
	title := r.GetName()
	if r.Namespace != "" {
		title = strings.Join([]string{r.Namespace, title}, "/")
	}
	return title
}

func (r Repository) GetSource() string {
	switch {
	case r.Src != "":
		return r.Src
	default:
		return r.Reason
	}
}

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
