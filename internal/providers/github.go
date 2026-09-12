package providers

import (
	"context"
	"fmt"
	"time"

	"github.com/shurcooL/githubv4"
	"golang.org/x/oauth2"

	"github.com/rafi/gits/domain"
)

const (
	// githubPageFloor is how many further pages the budget must still afford
	// before pacing engages — enough headroom to finish nearly any account
	// back-to-back. It counts pages rather than points so that a costlier
	// query paces sooner without anyone retuning the constant.
	githubPageFloor = 100

	// githubResetPad is added to a wait that runs to the end of the window,
	// so the next request lands after the budget is restored rather than on
	// the boundary.
	githubResetPad = time.Second

	// githubPageSize is how many repositories one GraphQL page asks for —
	// GitHub's maximum, so an account costs as few pages as it can.
	githubPageSize = 100
)

type gitHubProvider struct {
	client          *githubv4.Client
	includeArchived bool

	// nowFunc reads the clock for rate-limit pacing; tests replace it.
	nowFunc func() time.Time
}

// githubRateLimit is the GraphQL rateLimit block: what the query just cost,
// how many points are left, and when the whole budget is restored. GitHub's
// GraphQL budget is points per hour — 5,000 of them, against which a page of
// this query costs one — and it is restored whole at ResetAt rather than
// trickled back.
type githubRateLimit struct {
	Cost      githubv4.Int
	Remaining githubv4.Int
	ResetAt   githubv4.DateTime
}

func (c *gitHubProvider) now() time.Time {
	if c.nowFunc == nil {
		return time.Now()
	}
	return c.nowFunc()
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
		RateLimit       githubRateLimit
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
		"count":      githubv4.Int(githubPageSize),
		"isArchived": isArchived,
		// Null as first argument to get first page.
		"cursor": (*githubv4.String)(nil),
	}

	repos := []domain.Repository{}
	ownerID := ""
	what := fmt.Sprintf("GitHub repositories for %q", ownerName)
	// The budget is read from the page just fetched, so each gap is priced
	// by the most recent thing GitHub said about it.
	pace := func() time.Duration { return rateLimitDelay(q.RateLimit, c.now()) }
	err := paginate(ctx, what, pace, func(int) (bool, error) {
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

// rateLimitDelay decides how long to wait before asking for another page,
// given the rateLimit block the last one returned.
//
// With pages to spare there is nothing to ration, so they run back-to-back.
// Under the floor the pages still affordable are spread evenly across what is
// left of the window, which slows a long walk down instead of stopping it
// dead. With none affordable there is no choice but to wait the window out.
func rateLimitDelay(rl githubRateLimit, now time.Time) time.Duration {
	pages := rl.Remaining / max(rl.Cost, 1)
	if pages >= githubPageFloor {
		return 0
	}
	// A zero ResetAt is a response that carried no budget at all; a past one
	// is a window that has already closed. Neither is worth waiting on.
	window := rl.ResetAt.Sub(now)
	if window <= 0 {
		return 0
	}
	if pages < 1 {
		return window + githubResetPad
	}
	return window / time.Duration(pages)
}
