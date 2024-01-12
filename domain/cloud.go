package domain

import "fmt"

// ProviderSource represents a configuration source of projects/repositories.
type ProviderSource struct {
	Type    string      `json:"type"`
	Search  SearchQuery `json:"search,omitempty"`
	Include []string    `json:"include,omitempty"`
	Exclude []string    `json:"exclude,omitempty"`
}

const readmeURL = "https://github.com/rafi/gits#config"

// GetFilterID returns a unique identifier for search.
func (p ProviderSource) GetFilterID() (string, error) {
	id := ""
	fieldName := ""
	errStr := fmt.Sprintf(`source %q error: %%q is missing. see %s`, p.Type, readmeURL)

	switch p.Type {
	case "github":
		fieldName = "owner"
		id = p.Search.Owner
	case "gitlab":
		fieldName = "groupID"
		id = p.Search.GroupID
	case "bitbucket":
		fieldName = "owner"
		id = p.Search.Owner
	case "filesystem":
		fieldName = "path"
		id = p.Search.Path
	default:
		return "", fmt.Errorf("unknown source type: %s. see %s", p.Type, readmeURL)
	}

	if id == "" {
		return "", fmt.Errorf(errStr, fieldName)
	}
	return id, nil
}

// SearchQuery represents a search query a git provider source.
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
