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

type gitProvider interface {
	LoadRepos(ctx context.Context, id string, gitClient git.GitClient, project *domain.Project) error
}

// Options carries user settings into provider construction. Token falls back
// to provider-specific environment variables when empty.
type Options struct {
	Token           string
	IncludeArchived bool
	Timeout         time.Duration
}

func NewGitProvider(providerName string, opts Options) (gitProvider, error) {
	switch Provider(providerName) {
	case ProviderGitHub:
		return newGitHubProvider(opts)
	case ProviderGitLab:
		return newGitLabProvider(opts)
	case ProviderBitbucket:
		return newBitbucketProvider(opts)
	case ProviderFilesystem:
		return newFilesystemProvider()
	default:
		return nil, fmt.Errorf("unknown provider: %s", providerName)
	}
}

func getFirstEnvValue(keys []string) string {
	for _, key := range keys {
		if os.Getenv(key) != "" {
			return os.Getenv(key)
		}
	}
	return ""
}
