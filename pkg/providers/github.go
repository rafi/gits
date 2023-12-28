package providers

import (
	"context"
	"fmt"

	"github.com/shurcooL/githubv4"
	"golang.org/x/oauth2"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/pkg/git"
)

var gitHubTokenEnvVarNames = []string{"GITHUB_TOKEN", "HOMEBREW_GITHUB_API_TOKEN"}

type gitHubProvider struct {
	client     *githubv4.Client
	sourceType Provider
}

func newGitHubProvider(token string) (*gitHubProvider, error) {
	provider := &gitHubProvider{sourceType: ProviderGitHub}
	if token == "" {
		token = findFirstEnvVar(gitHubTokenEnvVarNames)
	}
	if token == "" {
		return provider, fmt.Errorf("token is required for %s", provider.sourceType)
	}

	src := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	httpClient := oauth2.NewClient(context.Background(), src)
	provider.client = githubv4.NewClient(httpClient)
	return provider, nil
}

func (c *gitHubProvider) LoadRepos(ownerName string, _ git.Git, project *domain.Project) (err error) {
	project.Repos, project.ID, err = c.fetchRepos(ownerName)
	if err != nil {
		return err
	}
	return nil
}

func (c *gitHubProvider) fetchRepos(ownerName string) ([]domain.Repository, string, error) {
	var q struct {
		Search struct {
			Edges []struct {
				Node struct {
					Repository struct {
						ID    githubv4.String
						Name  githubv4.String
						Owner struct {
							ID    githubv4.String
							Login githubv4.String
						}
						Description githubv4.String
						URL         githubv4.String
						SSHURL      githubv4.String
					} `graphql:"... on Repository"`
				}
			}
			RepositoryCount githubv4.Int
		} `graphql:"search(first: $count, query: $searchQuery, type: REPOSITORY)"`
	}

	variables := map[string]interface{}{
		"searchQuery": githubv4.String(
			fmt.Sprintf(`org:%s`, githubv4.String(ownerName)),
		),
		"count": githubv4.Int(100),
	}

	repos := []domain.Repository{}
	ownerID := ""

	ctx := context.Background()
	err := c.client.Query(ctx, &q, variables)
	if err != nil {
		return repos, ownerID, err
	}

	if len(q.Search.Edges) == 0 || q.Search.RepositoryCount == 0 {
		return repos, ownerID, fmt.Errorf("no repositories found")
	}

	ownerID = string(q.Search.Edges[0].Node.Repository.Owner.ID)
	for _, edge := range q.Search.Edges {
		node := edge.Node.Repository
		repos = append(repos, domain.Repository{
			ID:        string(node.ID),
			Name:      string(node.Name),
			Namespace: string(node.Owner.Login),
			Src:       string(node.SSHURL),
			URL:       string(node.URL),
			Desc:      string(node.Description),
		})
	}

	// TODO: pagination

	return repos, ownerID, nil
}
