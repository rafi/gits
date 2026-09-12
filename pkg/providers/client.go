package providers

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/pkg/git"
)

type Provider string

const (
	ProviderGitHub     Provider = "github"
	ProviderGitLab     Provider = "gitlab"
	ProviderBitbucket  Provider = "bitbucket"
	ProviderFilesystem Provider = "filesystem"
)

// tokenEnvVarNames are the per-provider environment variable fallbacks for
// Options.Token. Providers absent from the table need no token.
var tokenEnvVarNames = map[Provider][]string{
	ProviderGitHub:    {"GITHUB_TOKEN", "HOMEBREW_GITHUB_API_TOKEN"},
	ProviderGitLab:    {"GITLAB_TOKEN"},
	ProviderBitbucket: {"BITBUCKET_TOKEN"},
}

type gitProvider interface {
	LoadRepos(ctx context.Context, id string, project *domain.Project) error
}

// Options carries user settings into provider construction. Token falls back
// to provider-specific environment variables when empty.
type Options struct {
	Token           string
	IncludeArchived bool
	Timeout         time.Duration
	GitClient       git.GitClient
}

func NewGitProvider(providerName string, opts Options) (gitProvider, error) {
	provider := Provider(providerName)
	if envVars, ok := tokenEnvVarNames[provider]; ok {
		if opts.Token == "" {
			opts.Token = getFirstEnvValue(envVars)
		}
		if opts.Token == "" {
			return nil, fmt.Errorf("token is required for %s", provider)
		}
	}
	switch provider {
	case ProviderGitHub:
		return newGitHubProvider(opts), nil
	case ProviderGitLab:
		return newGitLabProvider(opts)
	case ProviderBitbucket:
		return newBitbucketProvider(opts)
	case ProviderFilesystem:
		return newFilesystemProvider(opts), nil
	default:
		return nil, fmt.Errorf("unknown provider: %s", providerName)
	}
}

// IsRemote reports whether a provider type lists repositories on a remote
// service rather than the local filesystem.
func IsRemote(providerType string) bool {
	return providerType != "" && Provider(providerType) != ProviderFilesystem
}

// HasCache reports whether a project source reads and writes a repository
// cache: only remote sources with a search filter do.
func HasCache(source *domain.ProviderSource) bool {
	return source != nil && source.Search != "" && IsRemote(source.Type)
}

func getFirstEnvValue(keys []string) string {
	for _, key := range keys {
		if os.Getenv(key) != "" {
			return os.Getenv(key)
		}
	}
	return ""
}
