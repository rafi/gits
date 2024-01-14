package domain

import (
	"fmt"
	"strings"
)

const readmeURL = "https://github.com/rafi/gits#config"

// ProviderSource represents a cloud-provider source of repositories.
type ProviderSource struct {
	Type   string `json:"type,omitempty"`
	Search string `json:"search,omitempty"`
}

// UniqueKey returns a unique key for a specific provider source.
func (ps ProviderSource) UniqueKey() string {
	searchKey := strings.ReplaceAll(ps.Search, "/", "%")
	return fmt.Sprintf("%s-%s", ps.Type, searchKey)
}

func (ps ProviderSource) Validate() error {
	fieldName := ""
	switch ps.Type {
	case "github":
		fieldName = "owner"
	case "gitlab":
		fieldName = "groupID"
	case "bitbucket":
		fieldName = "owner"
	case "filesystem":
		fieldName = "path"
	default:
		return fmt.Errorf("unknown source type: %s. see %s", ps.Type, readmeURL)
	}
	if ps.Search == "" {
		return fmt.Errorf(
			"for %s provider, make sure you included the correct %q value"+
				" in your config file under the `search:` key.\nsee %s",
			ps.Type,
			fieldName,
			readmeURL,
		)
	}
	return nil
}
