package providers

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/infra/git"
)

// Provider names a Provider Source type. The strings are config vocabulary.
type Provider string

// The Provider Source types a Project may be discovered from.
const (
	ProviderGitHub     Provider = domain.ProviderGitHub
	ProviderGitLab     Provider = domain.ProviderGitLab
	ProviderBitbucket  Provider = domain.ProviderBitbucket
	ProviderGitea      Provider = domain.ProviderGitea
	ProviderForgejo    Provider = domain.ProviderForgejo
	ProviderGerrit     Provider = domain.ProviderGerrit
	ProviderFilesystem Provider = domain.ProviderFilesystem
)

// GitProvider discovers the repositories of one Provider Source.
type GitProvider interface {
	// LoadRepos fills project with the repositories id names.
	LoadRepos(ctx context.Context, id string, project *domain.Project) error
}

// constructors builds each domain.ProviderType, keyed by its Name.
var constructors = map[string]func(Options) (GitProvider, error){
	domain.ProviderGitHub:     infallible(newGitHubProvider),
	domain.ProviderGitLab:     fallible(newGitLabProvider),
	domain.ProviderBitbucket:  fallible(newBitbucketProvider),
	domain.ProviderGitea:      fallible(giteaConstructor(domain.ProviderGitea)),
	domain.ProviderForgejo:    fallible(giteaConstructor(domain.ProviderForgejo)),
	domain.ProviderGerrit:     fallible(newGerritProvider),
	domain.ProviderFilesystem: infallible(newFilesystemProvider),
}

// fallible adapts a constructor that can fail, keeping a failed construction
// a nil interface rather than one holding a nil pointer.
func fallible[P GitProvider](newProvider func(Options) (P, error)) func(Options) (GitProvider, error) {
	return func(opts Options) (GitProvider, error) {
		p, err := newProvider(opts)
		if err != nil {
			return nil, err
		}
		return p, nil
	}
}

// infallible adapts a constructor that cannot fail.
func infallible[P GitProvider](newProvider func(Options) P) func(Options) (GitProvider, error) {
	return func(opts Options) (GitProvider, error) {
		return newProvider(opts), nil
	}
}

// Options carries user settings into provider construction. When Token is
// empty, TokenCommand is executed to obtain one; when both are empty, the
// token falls back to provider-specific environment variables.
type Options struct {
	// Log traces provider work — page fetches, token command runs — for `-v`.
	// Nothing a user must read goes here.
	Log          *slog.Logger
	Token        string
	TokenCommand string
	// BaseURL is the self-hosted forge's web host (domain.ProviderSource
	// BaseURL); empty for the public host. Environment tokens are never
	// used with it.
	BaseURL string
	// Username is the account a token authenticates as and clones use
	// (domain.Settings.SourceUsername). Read by gerrit only.
	Username        string
	IncludeArchived bool
	Timeout         time.Duration
	// RateLimit caps requests per second to one host, shared with every
	// provider on it; 0 is unlimited.
	RateLimit float64
	// GitClient is read-only: provider discovery only ever asks whether a
	// path is a repository.
	GitClient git.Reader
}

// httpClient returns the HTTP client for a provider of typeName: bounded by
// Timeout, paced by RateLimit, and retrying throttled requests when the type
// is retry-safe.
func (o Options) httpClient(typeName string) *http.Client {
	providerType, _ := domain.LookupProviderType(typeName)
	return &http.Client{
		Timeout: o.Timeout,
		Transport: newLimitedTransport(
			http.DefaultTransport, o.RateLimit, providerType.RetrySafe, o.Log),
	}
}

// NewGitProvider returns the provider for providerName, resolving its token
// first. A provider that does not require one runs anonymously without it.
func NewGitProvider(ctx context.Context, providerName string, opts Options) (GitProvider, error) {
	providerType, ok := domain.LookupProviderType(providerName)
	newProvider, known := constructors[providerName]
	if !ok || !known {
		return nil, fmt.Errorf("unknown provider: %s", providerName)
	}
	var err error
	opts.Token, err = resolveToken(ctx, providerType, opts)
	if err != nil {
		return nil, err
	}
	if providerType.TokenRequired && opts.Token == "" {
		if opts.BaseURL != "" {
			return nil, fmt.Errorf(
				"token is required for %s at %s: set token or tokenCommand"+
					" (environment variables apply only to the public host)",
				providerName, opts.BaseURL)
		}
		return nil, fmt.Errorf("token is required for %s", providerName)
	}
	return newProvider(opts)
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
