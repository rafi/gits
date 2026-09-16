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
	// URL is the web host of a self-hosted forge; see BaseURL. A user in it
	// is a credential, stripped with the rest.
	URL string `json:"url,omitempty"`
	// Username is the account a gerrit source authenticates and clones as;
	// see Settings.SourceUsername. Not a secret, so kept when serialized.
	Username string `json:"username,omitempty"`

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

// WithoutAuth returns the source with its credentials, the url's user
// included, removed.
func (ps ProviderSource) WithoutAuth() ProviderSource {
	ps.Token, ps.TokenCommand, ps.TokenCmd = "", "", ""
	ps.URL = urlWithoutUser(ps.URL)
	return ps
}

// UniqueKey returns the cache key for the source. A source on a self-hosted
// host keys by that host too. Sources with their own credentials, a url user
// or a username get a distinct key, derived from a digest of them.
func (ps ProviderSource) UniqueKey() string {
	key := ps.Type
	if base := ps.BaseURL(); base != "" {
		host := base[strings.Index(base, "://")+len("://"):]
		key += "-" + strings.NewReplacer("/", "%", ":", "_").Replace(host)
	}
	key += "-" + strings.ReplaceAll(ps.Search, "/", "%")
	auth, user := ps.Auth(), ps.URLUser()
	if !auth.IsZero() || user != "" || ps.Username != "" {
		key += "-" + authDigest(auth, user, ps.Username)
	}
	return key
}

// authDigest returns a short hash of auth, a url user and a source username
// that does not reveal them.
func authDigest(auth ProviderSettings, urlUser, username string) string {
	const digestChars = 12
	// Separator prevents collisions between fields.
	material := auth.Token + "\x00" + auth.Command()
	// Each only when set, so keys of sources without one are unchanged.
	if urlUser != "" {
		material += "\x00" + urlUser
	}
	if username != "" {
		material += "\x00username\x00" + username
	}
	sum := sha256.Sum256([]byte(material))
	return hex.EncodeToString(sum[:])[:digestChars]
}

// Validate reports whether the Provider Source names a known type, carries
// the search term that type requires, and has a url the type accepts.
func (ps ProviderSource) Validate() error {
	providerType, ok := LookupProviderType(ps.Type)
	if !ok {
		return fmt.Errorf("unknown source type: %s. see %s", ps.Type, readmeURL)
	}
	if ps.Search == "" {
		return fmt.Errorf(
			"for %s provider, make sure you included the correct %q value"+
				" in your config file under the `search:` key.\nsee %s",
			ps.Type,
			providerType.SearchField,
			readmeURL,
		)
	}
	if ps.Username != "" && !providerType.UsernameAllowed {
		return fmt.Errorf("a %s source does not take `username:`. see %s", providerType.Name, readmeURL)
	}
	return checkSourceURL(providerType, ps.URL)
}
