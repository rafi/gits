package providers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	gitea "gitea.dev/sdk"

	"github.com/rafi/gits/domain"
)

// giteaPageSize is how many repositories one page asks for. Servers cap it
// at their MAX_RESPONSE_ITEMS, and pagination follows their Link header, so
// a lower cap costs pages, not repositories.
const giteaPageSize = 50

// errGiteaNotFound marks a listing the server answered with 404: the owner
// is not of the kind asked for, or is invisible to these credentials.
var errGiteaNotFound = errors.New("not found")

// giteaProvider discovers from a Gitea or Forgejo host. Forgejo keeps
// Gitea's listing endpoints, so both type names share it; only the listing
// endpoints are used, which the SDK does not gate on a server version it
// would misread from Forgejo.
type giteaProvider struct {
	client *gitea.Client
	// typeName is the Provider Source type, "gitea" or "forgejo".
	typeName        string
	log             *slog.Logger
	includeArchived bool
}

// giteaConstructor returns the constructor for the Gitea-API type typeName.
func giteaConstructor(typeName string) func(Options) (*giteaProvider, error) {
	return func(opts Options) (*giteaProvider, error) {
		return newGiteaProvider(typeName, opts)
	}
}

// newGiteaProvider builds a client for opts.BaseURL, anonymous unless a
// token resolved.
func newGiteaProvider(typeName string, opts Options) (*giteaProvider, error) {
	if opts.BaseURL == "" {
		return nil, fmt.Errorf("url is required for %s", typeName)
	}
	clientOpts := []gitea.ClientOption{gitea.SetHTTPClient(opts.httpClient(typeName))}
	if opts.Token != "" {
		clientOpts = append(clientOpts, gitea.SetToken(opts.Token))
	}
	client, err := gitea.NewClient(opts.BaseURL, clientOpts...)
	if err != nil {
		return nil, fmt.Errorf("unable to create %s client: %w", typeName, err)
	}
	return &giteaProvider{
		client:          client,
		typeName:        typeName,
		log:             opts.Log,
		includeArchived: opts.IncludeArchived,
	}, nil
}

// LoadRepos lists the repositories of owner, an organization or, failing
// that, a user. The result is flat: Gitea has no nested groups.
func (c *giteaProvider) LoadRepos(ctx context.Context, owner string, project *domain.Project) error {
	project.ID = owner

	repos, err := c.fetchRepos(ctx, owner, "organization",
		func(opt gitea.ListOptions) ([]*gitea.Repository, *gitea.Response, error) {
			return c.client.Repositories.ListOrgRepos(ctx, owner, gitea.ListOrgReposOptions{ListOptions: opt})
		})
	if errors.Is(err, errGiteaNotFound) {
		repos, err = c.fetchRepos(ctx, owner, "user",
			func(opt gitea.ListOptions) ([]*gitea.Repository, *gitea.Response, error) {
				return c.client.Repositories.ListUserRepos(ctx, owner, gitea.ListReposOptions{ListOptions: opt})
			})
	}
	if errors.Is(err, errGiteaNotFound) {
		return fmt.Errorf("%s: no organization or user named %q", c.typeName, owner)
	}
	if err != nil {
		return err
	}
	project.Repos = repos
	return nil
}

// fetchRepos walks every page of one listing, keeping the repositories
// worth cloning. A 404 on the first page is errGiteaNotFound, so the caller
// can try another kind of owner.
func (c *giteaProvider) fetchRepos(
	ctx context.Context,
	owner, kind string,
	list func(gitea.ListOptions) ([]*gitea.Repository, *gitea.Response, error),
) ([]domain.Repository, error) {
	repos := []domain.Repository{}
	what := fmt.Sprintf("%s repositories of %s %s", c.typeName, kind, owner)
	err := paginate(ctx, c.log, what, constantPause(0), func(page int) (bool, error) {
		items, resp, err := list(gitea.ListOptions{Page: page, PageSize: giteaPageSize})
		if page == 1 && resp != nil && resp.StatusCode == http.StatusNotFound {
			return false, errGiteaNotFound
		}
		if err != nil {
			return false, fmt.Errorf("%s: list repos of %s %q: %w", c.typeName, kind, owner, err)
		}
		for _, item := range items {
			if skipGiteaRepo(item, c.includeArchived) {
				continue
			}
			repos = append(repos, giteaRepository(item))
		}
		return resp != nil && resp.NextPage != 0, nil
	})
	if err != nil {
		return nil, err
	}
	return repos, nil
}

// skipGiteaRepo reports whether a listed repository is omitted: empty ones
// always (nothing to clone), archived ones unless settings.includeArchived
// is set.
func skipGiteaRepo(r *gitea.Repository, includeArchived bool) bool {
	return r.Empty || (r.Archived && !includeArchived)
}

// giteaRepository converts a listed repository.
func giteaRepository(r *gitea.Repository) domain.Repository {
	repo := domain.Repository{
		ID:   strconv.FormatInt(r.ID, 10),
		Name: r.Name,
		Src:  r.SSHURL,
		URL:  r.HTMLURL,
		Desc: r.Description,
	}
	if r.Owner != nil {
		repo.Namespace = r.Owner.UserName
	}
	return repo
}
