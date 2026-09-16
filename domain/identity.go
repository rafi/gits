package domain

// RepoIdentity is a repository's identity as it appears on the wire: the
// fields that describe *which* repository this is, with nothing about its
// local presence.
//
// It exists so the JSON layer projects these fields rather than restating
// them. Repo State is deliberately absent — it is computed per command and
// must stay out of the cache file and out of Project.CalculateHash, both of
// which marshal the domain types.
type RepoIdentity struct {
	ID        string `json:"id,omitempty"`
	Name      string `json:"name,omitempty"`
	Namespace string `json:"namespace,omitempty"`
	Src       string `json:"src,omitempty"`
	Dir       string `json:"dir,omitempty"`
	URL       string `json:"url,omitempty"`
	Desc      string `json:"desc,omitempty"`
	// Tags are the repository's own plus inherited project tags.
	Tags []Tag `json:"tags,omitempty"`
}

// Identity returns the repository's wire identity.
func (r Repository) Identity() RepoIdentity {
	return RepoIdentity{
		ID:        r.ID,
		Name:      r.Name,
		Namespace: r.Namespace,
		Src:       r.Src,
		Dir:       r.Dir,
		URL:       r.URL,
		Desc:      r.Desc,
		Tags:      r.Tags,
	}
}

// ProjectIdentity is a project node's identity on the wire, without its
// repositories or sub-projects — the caller fills those, since it knows
// whether it is descending.
//
// Include and Exclude are not here: they are emitted after a project's repos
// and subprojects, so they belong to the wire type that owns those fields.
type ProjectIdentity struct {
	Source *ProviderSource `json:"source,omitempty"`
	Clone  *bool           `json:"clone,omitempty"`
	ID     string          `json:"id"`
	Name   string          `json:"name"`
	Path   string          `json:"path"`
	Desc   string          `json:"desc,omitempty"`
	// Tags are the project's own tags, not merged with its parents'.
	Tags []Tag `json:"tags,omitempty"`
}

// Identity returns the project node's wire identity. The Provider Source is
// included without credentials.
func (p *Project) Identity() ProjectIdentity {
	source := p.Source
	if source != nil {
		redacted := source.WithoutAuth()
		source = &redacted
	}
	return ProjectIdentity{
		Source: source,
		Clone:  p.Clone,
		ID:     p.ID,
		Name:   p.Name,
		Path:   p.Path,
		Desc:   p.Desc,
		Tags:   p.Tags,
	}
}
