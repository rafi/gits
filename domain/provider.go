package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

const readmeURL = "https://github.com/rafi/gits#configuration"

// ProviderSource represents a cloud-provider source of repositories.
type ProviderSource struct {
	Type   string `json:"type,omitempty"`
	Search string `json:"search,omitempty"`

	// Credentials; stripped before serialization, see WithoutAuth.
	Token        string `json:"token,omitempty"`
	TokenCommand string `json:"tokenCommand,omitempty"`
	TokenCmd     string `json:"token-cmd,omitempty"`
}

// Auth returns the source's own credentials. The zero value means none.
func (ps ProviderSource) Auth() ProviderSettings {
	return ProviderSettings{
		Token:        ps.Token,
		TokenCommand: ps.TokenCommand,
		TokenCmd:     ps.TokenCmd,
	}
}

// WithoutAuth returns the source with its credentials removed.
func (ps ProviderSource) WithoutAuth() ProviderSource {
	ps.Token, ps.TokenCommand, ps.TokenCmd = "", "", ""
	return ps
}

// UniqueKey returns the cache key for the source. Sources with their own
// credentials get a distinct key, derived from a digest of them.
func (ps ProviderSource) UniqueKey() string {
	searchKey := strings.ReplaceAll(ps.Search, "/", "%")
	key := fmt.Sprintf("%s-%s", ps.Type, searchKey)
	if auth := ps.Auth(); !auth.IsZero() {
		key += "-" + authDigest(auth)
	}
	return key
}

// authDigest returns a short hash of auth that does not reveal it.
func authDigest(auth ProviderSettings) string {
	const digestChars = 12
	// Separator prevents collisions between fields.
	sum := sha256.Sum256([]byte(auth.Token + "\x00" + auth.Command()))
	return hex.EncodeToString(sum[:])[:digestChars]
}

// Validate reports whether the Provider Source names a known type and
// carries the search term that type requires.
func (ps ProviderSource) Validate() error {
	var fieldName string
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
