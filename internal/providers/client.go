package providers

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/git"
)

// Provider names a Provider Source type. The strings are config vocabulary.
type Provider string

// The Provider Source types a Project may be discovered from.
const (
	ProviderGitHub     Provider = "github"
	ProviderGitLab     Provider = "gitlab"
	ProviderBitbucket  Provider = "bitbucket"
	ProviderFilesystem Provider = "filesystem"
)

// tokenEnvVarNames are the per-provider environment variable fallbacks for
// Options.Token. A provider with no names needs no token at all.
var tokenEnvVarNames = map[Provider][]string{
	ProviderGitHub:    {"GITHUB_TOKEN", "HOMEBREW_GITHUB_API_TOKEN"},
	ProviderGitLab:    {"GITLAB_TOKEN"},
	ProviderBitbucket: {"BITBUCKET_TOKEN"},
	// A filesystem project is read from disk and authenticates against
	// nothing, so it has no environment fallback to name.
	ProviderFilesystem: nil,
}

type gitProvider interface {
	LoadRepos(ctx context.Context, id string, project *domain.Project) error
}

// Options carries user settings into provider construction. When Token is
// empty, TokenCommand is executed to obtain one; when both are empty, the
// token falls back to provider-specific environment variables.
type Options struct {
	// Log traces provider work — page fetches, token command runs — for `-v`.
	// Nothing a user must read goes here.
	Log             *slog.Logger
	Token           string
	TokenCommand    string
	IncludeArchived bool
	Timeout         time.Duration
	// GitClient is read-only: provider discovery only ever asks whether a
	// path is a repository.
	GitClient git.Reader
}

// NewGitProvider returns the provider for providerName, resolving its token
// first when that provider needs one.
func NewGitProvider(ctx context.Context, providerName string, opts Options) (gitProvider, error) {
	provider := Provider(providerName)
	if names := tokenEnvVarNames[provider]; len(names) > 0 {
		var err error
		opts.Token, err = resolveToken(ctx, provider, opts)
		if err != nil {
			return nil, err
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

// getFirstEnvValue returns the value of the first key set to a non-empty value,
// or "" when none of them is.
func getFirstEnvValue(keys []string) string {
	for _, key := range keys {
		if value, ok := os.LookupEnv(key); ok && value != "" {
			return value
		}
	}
	return ""
}
