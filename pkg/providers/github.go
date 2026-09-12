package providers

import (
	"context"
	"fmt"
	"time"

	"github.com/shurcooL/githubv4"
	"golang.org/x/oauth2"

	"github.com/rafi/gits/domain"
)

// githubPageDelay softens the request rate between pages.
const githubPageDelay = 100 * time.Millisecond

type gitHubProvider struct {
	client          *githubv4.Client
	includeArchived bool
}

func newGitHubProvider(opts Options) *gitHubProvider {
	src := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: opts.Token},
	)
	httpClient := oauth2.NewClient(context.Background(), src)
	if opts.Timeout > 0 {
		httpClient.Timeout = opts.Timeout
	}
	return &gitHubProvider{
		client:          githubv4.NewClient(httpClient),
		includeArchived: opts.IncludeArchived,
	}
}

func (c *gitHubProvider) LoadRepos(ctx context.Context, ownerName string, project *domain.Project) (err error) {
	project.Repos, project.ID, err = c.fetchRepos(ctx, ownerName)
	return err
}

// fetchRepos lists every repository of a user or organization via the
// repositoryOwner connection, which unlike the search API has no 1,000
// result cap and covers both account types with one query.
func (c *gitHubProvider) fetchRepos(ctx context.Context, ownerName string) ([]domain.Repository, string, error) {
	var q struct {
		RepositoryOwner *struct {
			ID           githubv4.String
			Login        githubv4.String
			Repositories struct {
				Nodes []struct {
					ID          githubv4.String
					Name        githubv4.String
					Description githubv4.String
					URL         githubv4.String
					SSHURL      githubv4.String
				}
				PageInfo struct {
					EndCursor   githubv4.String
					HasNextPage bool
				}
			} `graphql:"repositories(first: $count, after: $cursor, isArchived: $isArchived, ownerAffiliations: OWNER)"`
		} `graphql:"repositoryOwner(login: $owner)"`
	}

	// isArchived=false filters archived repositories server-side; null
	// lifts the filter. The explicit OWNER affiliation matters too: the
	// API default also includes repos the owner merely collaborates on.
	var isArchived *githubv4.Boolean
	if !c.includeArchived {
		isArchived = githubv4.NewBoolean(false)
	}
	vars := map[string]any{
		"owner":      githubv4.String(ownerName),
		"count":      githubv4.Int(100),
		"isArchived": isArchived,
		// Null as first argument to get first page.
		"cursor": (*githubv4.String)(nil),
	}

	repos := []domain.Repository{}
	ownerID := ""
	what := fmt.Sprintf("GitHub repositories for %q", ownerName)
	err := paginate(ctx, what, githubPageDelay, func(int) (bool, error) {
		if err := c.client.Query(ctx, &q, vars); err != nil {
			return false, err
		}
		owner := q.RepositoryOwner
		if owner == nil {
			return false, fmt.Errorf(
				"%q is not a known github user or organization", ownerName)
		}

		for _, node := range owner.Repositories.Nodes {
			repos = append(repos, domain.Repository{
				ID:        string(node.ID),
				Name:      string(node.Name),
				Namespace: string(owner.Login),
				Src:       string(node.SSHURL),
				URL:       string(node.URL),
				Desc:      string(node.Description),
			})
		}
		if !owner.Repositories.PageInfo.HasNextPage {
			ownerID = string(owner.ID)
			return false, nil
		}
		vars["cursor"] = githubv4.NewString(owner.Repositories.PageInfo.EndCursor)
		return true, nil
	})
	return repos, ownerID, err
}
