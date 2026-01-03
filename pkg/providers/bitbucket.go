package providers

import (
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

func (c *bitbucketProvider) LoadRepos(ownerName string, _ git.Git, project *domain.Project) error {
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

	ownerID := ownerName
	if len(result.Items) > 0 {
		ownerID = result.Items[0].Owner["uuid"].(string)
	}

	repos := []domain.Repository{}
	for _, item := range result.Items {
		repo := domain.Repository{
			ID:        item.Uuid,
			Name:      item.Slug,
			Namespace: ownerName,
			Desc:      item.Description,
		}

		links := item.Links["clone"].([]interface{})
		for _, link := range links {
			linkName := link.(map[string]interface{})["name"]
			linkHRef := link.(map[string]interface{})["href"]
			switch linkName {
			case "ssh":
				repo.Src = linkHRef.(string)
			case "https":
				repo.URL = linkHRef.(string)
			}
		}
		repos = append(repos, repo)
	}
	return repos, ownerID, nil
}
