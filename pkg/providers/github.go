package providers

import (
	"context"
	"fmt"
	"time"

	"github.com/shurcooL/githubv4"
	log "github.com/sirupsen/logrus"
	"golang.org/x/oauth2"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/pkg/git"
)

var gitHubTokenEnvVarNames = []string{
	"GITHUB_TOKEN",
	"HOMEBREW_GITHUB_API_TOKEN",
}

type gitHubProvider struct {
	client     *githubv4.Client
	sourceType Provider
}

func newGitHubProvider(token string) (*gitHubProvider, error) {
	provider := &gitHubProvider{sourceType: ProviderGitHub}
	if token == "" {
		token = getFirstEnvValue(gitHubTokenEnvVarNames)
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
	if len(project.Repos) == 0 {
		return fmt.Errorf("no repositories found")
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
						IsArchived  githubv4.Boolean
					} `graphql:"... on Repository"`
				}
			}
			PageInfo struct {
				EndCursor   githubv4.String
				HasNextPage bool
			}
			RepositoryCount githubv4.Int
		} `graphql:"search(first: $count, after: $cursor, query: $query, type: REPOSITORY)"`
	}

	searchQuery := map[string]interface{}{
		"query": githubv4.String(
			fmt.Sprintf(`org:%s`, githubv4.String(ownerName)),
		),
		"count": githubv4.Int(100),
		// Null as first argument to get first page.
		"cursor": (*githubv4.String)(nil),
	}

	ownerID := ""
	repos := []domain.Repository{}
	ctx := context.Background()
	pageNum := 0
	for {
		pageNum++
		log.Infof("Fetching GitHub repositories for %q (%d)…", ownerName, pageNum)

		err := c.client.Query(ctx, &q, searchQuery)
		if err != nil {
			return repos, ownerID, err
		}

		if len(q.Search.Edges) == 0 || q.Search.RepositoryCount == 0 {
			break
		}
		if ownerID == "" {
			ownerID = string(q.Search.Edges[0].Node.Repository.Owner.ID)
		}

		for _, edge := range q.Search.Edges {
			repo := edge.Node.Repository
			if repo.IsArchived {
				continue
			}
			repos = append(repos, domain.Repository{
				ID:        string(repo.ID),
				Name:      string(repo.Name),
				Namespace: string(repo.Owner.Login),
				Src:       string(repo.SSHURL),
				URL:       string(repo.URL),
				Desc:      string(repo.Description),
			})
		}
		if !q.Search.PageInfo.HasNextPage {
			break
		}
		searchQuery["cursor"] = githubv4.NewString(q.Search.PageInfo.EndCursor)
		time.Sleep(time.Millisecond * 100)
	}
	return repos, ownerID, nil
}
