package providers

import (
	"context"
	"fmt"
	"strings"

	"github.com/ktrysmt/go-bitbucket"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/pkg/git"
)

var bitbucketTokenEnvVarNames = []string{"BITBUCKET_TOKEN"}

type bitbucketProvider struct {
	client     *bitbucket.Client
	sourceType Provider
}

func newBitbucketProvider(token string) (*bitbucketProvider, error) {
	provider := &bitbucketProvider{sourceType: ProviderBitbucket}
	if token == "" {
		token = getFirstEnvValue(bitbucketTokenEnvVarNames)
	}
	if token == "" {
		return provider, fmt.Errorf("token is required for %s", provider.sourceType)
	}
	userLogin := strings.SplitN(token, ":", 2)
	if len(userLogin) != 2 {
		return provider, fmt.Errorf("token is invalid for %s", provider.sourceType)
	}
	var err error
	provider.client, err = bitbucket.NewBasicAuth(userLogin[0], userLogin[1])
	if err != nil {
		return provider, fmt.Errorf("bitbucket auth failed: %w", err)
	}
	provider.client.LimitPages = 1
	return provider, nil
}

func (c *bitbucketProvider) LoadRepos(_ context.Context, ownerName string, _ git.GitClient, project *domain.Project) error {
	var err error
	project.Repos, project.ID, err = c.fetchRepos(ownerName)
	if err != nil {
		return err
	}
	return nil
}

func (c *bitbucketProvider) fetchRepos(ownerName string) ([]domain.Repository, string, error) {
	listOpts := &bitbucket.RepositoriesOptions{Owner: ownerName}
	result, err := c.client.Repositories.ListForAccount(listOpts)
	if err != nil {
		return nil, "", err
	}
	repos, ownerID := parseRepos(result.Items, ownerName)
	return repos, ownerID, nil
}

// parseRepos converts untrusted Bitbucket API items into repositories. The
// Owner and Links maps are decoded from JSON as map[string]any, so every type
// assertion is comma-ok and a mismatch skips that field rather than panicking
// the whole process.
func parseRepos(items []bitbucket.Repository, ownerName string) ([]domain.Repository, string) {
	ownerID := ownerName
	if len(items) > 0 {
		if uuid, ok := items[0].Owner["uuid"].(string); ok {
			ownerID = uuid
		}
	}

	repos := []domain.Repository{}
	for _, item := range items {
		repo := domain.Repository{
			ID:        item.Uuid,
			Name:      item.Slug,
			Namespace: ownerName,
			Desc:      item.Description,
		}

		links, ok := item.Links["clone"].([]any)
		if !ok {
			repos = append(repos, repo)
			continue
		}
		for _, link := range links {
			linkMap, ok := link.(map[string]any)
			if !ok {
				continue
			}
			href, ok := linkMap["href"].(string)
			if !ok {
				continue
			}
			switch linkMap["name"] {
			case "ssh":
				repo.Src = href
			case "https":
				repo.URL = href
			}
		}
		repos = append(repos, repo)
	}
	return repos, ownerID
}
