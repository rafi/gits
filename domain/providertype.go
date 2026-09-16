package domain

import "slices"

// The Provider Source type names. The strings are config vocabulary.
const (
	ProviderGitHub     = "github"
	ProviderGitLab     = "gitlab"
	ProviderBitbucket  = "bitbucket"
	ProviderGitea      = "gitea"
	ProviderForgejo    = "forgejo"
	ProviderFilesystem = "filesystem"
)

// ProviderType describes one Provider Source type: everything validation,
// settings and token lookup need to know about it. Its constructor lives
// with the provider implementations, keyed by the same Name.
type ProviderType struct {
	// Name is the `type:` value selecting it.
	Name string
	// SearchField names what `search:` holds, for validation messages.
	SearchField string
	// TokenEnvVars are the environment variables consulted, in order, when
	// no token is configured.
	TokenEnvVars []string
	// TokenRequired rejects discovery when no token resolves.
	TokenRequired bool
	// Settings returns the type's `settings:` block; nil when it has none.
	Settings func(Settings) ProviderSettings
	// URLAllowed accepts `url:`, the web host of a self-hosted forge.
	URLAllowed bool
	// URLRequired rejects a source without `url:`; the type has no public
	// host to fall back to.
	URLRequired bool
	// PublicHost is the normalized url meaning the same as no url at all.
	PublicHost string
	// RetrySafe lets a throttled request be sent again. Declared rather than
	// inferred from the method: GitHub reads over POST.
	RetrySafe bool
}

// searchOwner labels a search naming a user or organization.
const searchOwner = "owner"

var providerTypes = []ProviderType{
	{
		Name:          ProviderGitHub,
		RetrySafe:     true,
		SearchField:   searchOwner,
		TokenEnvVars:  []string{"GITHUB_TOKEN", "HOMEBREW_GITHUB_API_TOKEN"},
		TokenRequired: true,
		Settings:      func(s Settings) ProviderSettings { return s.GitHub },
		URLAllowed:    true,
		PublicHost:    "https://github.com",
	},
	{
		Name:          ProviderGitLab,
		RetrySafe:     true,
		SearchField:   "groupID",
		TokenEnvVars:  []string{"GITLAB_TOKEN"},
		TokenRequired: true,
		Settings:      func(s Settings) ProviderSettings { return s.GitLab },
		URLAllowed:    true,
		PublicHost:    "https://gitlab.com",
	},
	{
		Name:          ProviderBitbucket,
		RetrySafe:     true,
		SearchField:   searchOwner,
		TokenEnvVars:  []string{"BITBUCKET_TOKEN"},
		TokenRequired: true,
		Settings:      func(s Settings) ProviderSettings { return s.Bitbucket },
	},
	{
		// Gitea and Forgejo have no public host, so no environment tokens,
		// and discover anonymously without a configured one.
		Name:        ProviderGitea,
		RetrySafe:   true,
		SearchField: searchOwner,
		Settings:    func(s Settings) ProviderSettings { return s.Gitea },
		URLAllowed:  true,
		URLRequired: true,
	},
	{
		Name:        ProviderForgejo,
		RetrySafe:   true,
		SearchField: searchOwner,
		Settings:    func(s Settings) ProviderSettings { return s.Forgejo },
		URLAllowed:  true,
		URLRequired: true,
	},
	{
		// Read from disk, so it authenticates against nothing.
		Name:        ProviderFilesystem,
		SearchField: "path",
	},
}

// LookupProviderType returns the Provider Source type called name.
func LookupProviderType(name string) (ProviderType, bool) {
	i := slices.IndexFunc(providerTypes, func(t ProviderType) bool { return t.Name == name })
	if i < 0 {
		return ProviderType{}, false
	}
	return providerTypes[i], true
}

// ProviderTypes returns every Provider Source type.
func ProviderTypes() []ProviderType {
	return slices.Clone(providerTypes)
}
