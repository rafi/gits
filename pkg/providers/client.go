package providers

import (
	"fmt"
	"os"

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
	LoadRepos(id string, gitClient git.Git, project *domain.Project) error
}

func NewGitProvider(providerName, token string) (gitProvider, error) {
	switch Provider(providerName) {
	case ProviderGitHub:
		return newGitHubProvider(token)
	case ProviderGitLab:
		return newGitLabProvider(token)
	case ProviderBitbucket:
		return newBitbucketProvider(token)
	case ProviderFilesystem:
		return newFilesystemProvider()
	default:
		return nil, fmt.Errorf("unknown provider: %s", providerName)
	}
}

func findFirstEnvVar(keys []string) string {
	for _, key := range keys {
		if os.Getenv(key) != "" {
			return os.Getenv(key)
		}
	}
	return ""
}
