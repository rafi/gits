package domain

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// parseSourceURL parses a `source.url`: an http(s) web host, optionally with
// a path and a user, never a password, query or fragment. The error never
// quotes raw, which may hold a password.
func parseSourceURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, errors.New("url is not a valid address")
	}
	if u.Scheme != "http" && u.Scheme != "https" || u.Host == "" {
		return nil, errors.New("url must be an http:// or https:// web host, e.g. https://git.example.com")
	}
	if _, hasPassword := u.User.Password(); hasPassword {
		return nil, errors.New("url must not carry a password; use token or tokenCommand")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("url must not carry a query or fragment")
	}
	return u, nil
}

// checkSourceURL reports whether raw is a url the provider type accepts.
func checkSourceURL(providerType ProviderType, raw string) error {
	if raw == "" {
		if providerType.URLRequired {
			return fmt.Errorf("a %s source requires `url:`, the forge's web address. see %s",
				providerType.Name, readmeURL)
		}
		return nil
	}
	if !providerType.URLAllowed {
		return fmt.Errorf("a %s source does not take `url:`. see %s", providerType.Name, readmeURL)
	}
	if _, err := parseSourceURL(raw); err != nil {
		return fmt.Errorf("for %s provider, %w", providerType.Name, err)
	}
	return nil
}

// BaseURL returns the forge host the source discovers from, without user or
// trailing slash, or "" for the type's public host. An unset or invalid url
// is "" too; Validate reports the latter.
func (ps ProviderSource) BaseURL() string {
	if ps.URL == "" {
		return ""
	}
	u, err := parseSourceURL(ps.URL)
	if err != nil {
		return ""
	}
	base := u.Scheme + "://" + strings.ToLower(u.Host) + strings.TrimRight(u.Path, "/")
	if providerType, ok := LookupProviderType(ps.Type); ok && base == providerType.PublicHost {
		return ""
	}
	return base
}

// URLUser returns the user named in the source url, if any.
func (ps ProviderSource) URLUser() string {
	if ps.URL == "" {
		return ""
	}
	u, err := parseSourceURL(ps.URL)
	if err != nil {
		return ""
	}
	return u.User.Username()
}

// urlWithoutUser returns raw with its user removed, or "" when raw cannot be
// parsed and so cannot be trusted to hold no credential.
func urlWithoutUser(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	u.User = nil
	return u.String()
}
