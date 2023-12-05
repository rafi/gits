package domain

// ProviderSource represents a configuration source of projects/repositories.
type ProviderSource struct {
	Type    string      `json:"type"`
	Search  SearchQuery `json:"search,omitempty"`
	Include []string    `json:"include,omitempty"`
	Exclude []string    `json:"exclude,omitempty"`
}

// GetFilterID returns a unique identifier for search.
func (p ProviderSource) GetFilterID() string {
	id := ""
	switch p.Type {
	case "github":
		id = p.Search.Owner
	case "gitlab":
		id = p.Search.GroupID
	case "bitbucket":
		id = p.Search.Owner
	case "filesystem":
		id = p.Search.Path
	}
	return id
}

// SearchQuery represents a search query params for a cloud-provider source.
type SearchQuery struct {
	Owner   string `json:"owner,omitempty"`
	GroupID string `json:"groupID,omitempty"`
	Path    string `json:"path,omitempty"`
}

func (s SearchQuery) String() string {
	out := ""
	if s.Owner != "" {
		out += "owner=" + s.Owner
	}
	if s.GroupID != "" {
		out += "groupID=" + s.GroupID
	}
	if s.Path != "" {
		out += "path=" + s.Path
	}
	return out
}
