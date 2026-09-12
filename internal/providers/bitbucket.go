// Package providers discovers a Project's repositories from its Provider
// Source — GitHub, GitLab, Bitbucket, or the filesystem.
package providers

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/ktrysmt/go-bitbucket"

	"github.com/rafi/gits/domain"
)

// tokenSeparator splits a Bitbucket basic-auth token into its two halves.
// Its presence is also what picks the authentication scheme: Atlassian
// documents an API token as either `email:token` over basic auth or the bare
// token as a bearer, and only the separator tells the two apart. Token shape
// is deliberately not guessed at — Atlassian does not document one.
const tokenSeparator = ":"

type bitbucketProvider struct {
	client *bitbucket.Client
	log    *slog.Logger
}

// newBitbucketProvider builds the API client from the configured token.
//
// Two forms are accepted, matching what Bitbucket Cloud accepts on the wire:
//
//	email:api-token    basic auth, the Atlassian account email as the user
//	api-token          bearer auth, which needs no email
//
// A legacy `user:app-password` is the same basic-auth request as the first
// form, so it keeps working for as long as Bitbucket honors it without a code
// path of its own. Atlassian has deprecated app passwords in favor of API
// tokens; see https://support.atlassian.com/bitbucket-cloud/docs/using-api-tokens/.
func newBitbucketProvider(opts Options) (*bitbucketProvider, error) {
	provider := &bitbucketProvider{log: opts.Log}
	var err error
	provider.client, err = newBitbucketClient(opts.Token)
	if err != nil {
		return nil, fmt.Errorf("bitbucket auth failed: %w", err)
	}
	// Pages are driven one at a time through the shared paginate() loop —
	// go-bitbucket's built-in auto-pager would fetch every page in one
	// blocking call, ignoring cancellation. A larger page size reduces
	// round-trips for big accounts.
	provider.client.DisableAutoPaging = true
	provider.client.Pagelen = 100
	// go-bitbucket's default HTTP client has no timeout, so a hung API
	// call would block forever.
	if opts.Timeout > 0 {
		provider.client.HttpClient.Timeout = opts.Timeout
	}
	return provider, nil
}

// newBitbucketClient returns the client for token, choosing basic or bearer
// authentication by whether the token carries a user half. Each half of a
// `user:token` pair must be non-empty: a bare separator names a credential
// neither scheme can send, and silently treating `:token` as a bearer would
// authenticate as somebody other than the user wrote down.
func newBitbucketClient(token string) (*bitbucket.Client, error) {
	user, secret, hasUser := strings.Cut(token, tokenSeparator)
	if !hasUser {
		if token == "" {
			return nil, fmt.Errorf("token is empty for %s", ProviderBitbucket)
		}
		// A bare token is an Atlassian API token used as a bearer, which
		// carries its own account and so needs no email.
		return bitbucket.NewOAuthbearerToken(token)
	}
	if user == "" || secret == "" {
		return nil, fmt.Errorf(
			"token is invalid for %s: expected `email:api-token`, or the API token alone",
			ProviderBitbucket)
	}
	return bitbucket.NewAPITokenAuth(user, secret)
}

func (c *bitbucketProvider) LoadRepos(ctx context.Context, ownerName string, project *domain.Project) error {
	var err error
	project.Repos, project.ID, err = c.fetchRepos(ctx, ownerName)
	if err != nil {
		return err
	}
	return nil
}

func (c *bitbucketProvider) fetchRepos(ctx context.Context, ownerName string) ([]domain.Repository, string, error) {
	var items []bitbucket.Repository
	what := fmt.Sprintf("Bitbucket repositories from %s", ownerName)
	err := paginate(ctx, c.log, what, constantPause(0), func(page int) (bool, error) {
		// go-bitbucket hardcodes context.Background internally, so honor
		// the caller's context before each page we request.
		if err := ctx.Err(); err != nil {
			return false, err
		}
		listOpts := &bitbucket.RepositoriesOptions{Owner: ownerName, Page: &page}
		result, err := c.client.Repositories.ListForAccount(listOpts)
		if err != nil {
			return false, fmt.Errorf("bitbucket: list repos for %q: %w", ownerName, err)
		}
		items = append(items, result.Items...)
		// A short (or empty) page is the last one.
		return len(result.Items) > 0 && len(result.Items) == int(result.Pagelen), nil
	})
	if err != nil {
		return nil, "", err
	}
	repos, ownerID := parseRepos(items, ownerName)
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
