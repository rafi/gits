package providers

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	retryablehttp "github.com/hashicorp/go-retryablehttp"
	gitlab "gitlab.com/gitlab-org/api/client-go"

	"github.com/rafi/gits/domain"
)

type gitLabProvider struct {
	client          *gitlab.Client
	log             *slog.Logger
	includeArchived bool
}

func newGitLabProvider(opts Options) (*gitLabProvider, error) {
	clientOpts := []gitlab.ClientOptionFunc{
		gitlab.WithHTTPClient(opts.httpClient(domain.ProviderGitLab)),
		gitlab.WithCustomRetry(gitLabRetryPolicy),
	}
	if opts.BaseURL != "" {
		clientOpts = append(clientOpts, gitlab.WithBaseURL(opts.BaseURL+"/api/v4"))
	}
	client, err := gitlab.NewClient(opts.Token, clientOpts...)
	if err != nil {
		return nil, fmt.Errorf("unable to create gitlab client: %w", err)
	}
	return &gitLabProvider{
		client:          client,
		log:             opts.Log,
		includeArchived: opts.IncludeArchived,
	}, nil
}

// gitLabRetryPolicy keeps the SDK's retries for server errors but leaves
// 429 and 503 to the transport, which already honored Retry-After for them;
// retrying those here too would multiply the attempts.
func gitLabRetryPolicy(ctx context.Context, resp *http.Response, err error) (bool, error) {
	if err == nil && resp != nil && throttled(resp.StatusCode) {
		return false, nil
	}
	return retryablehttp.DefaultRetryPolicy(ctx, resp, err)
}

// skipGitLabProject reports whether a listed project is omitted: empty
// repositories always (nothing to clone), archived ones unless
// settings.includeArchived is set.
func skipGitLabProject(p *gitlab.Project, includeArchived bool) bool {
	return p.EmptyRepo || (p.Archived && !includeArchived)
}

// gitLabPageSize is how many projects one keyset page asks for.
const gitLabPageSize = 50

var gitLabListOptions = gitlab.ListOptions{
	OrderBy:    "id",
	Pagination: "keyset",
	PerPage:    gitLabPageSize,
	Sort:       "asc",
}

// LoadRepos discovers a GitLab group's whole tree with two paginated walks:
// one over the group's descendant groups, and one over its projects with
// include_subgroups=true. Each project names its owning group in
// Namespace.FullPath, so the Sub-project tree is rebuilt client-side instead
// of costing a request pair per group.
func (c *gitLabProvider) LoadRepos(ctx context.Context, groupID string, project *domain.Project) error {
	project.ID = groupID

	root, _, err := c.client.Groups.GetGroup(groupID, nil, gitlab.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("gitlab: get group %q: %w", groupID, err)
	}
	if project.Name == "" {
		project.Name = root.Name
	}

	tree := newRepoTree(root.FullPath)
	groups, err := c.fetchDescendantGroups(ctx, groupID)
	if err != nil {
		return err
	}
	tree.addGroups(groups)

	repos, err := c.fetchGroupProjects(ctx, groupID)
	if err != nil {
		return err
	}
	tree.addRepos(repos)

	project.Repos = tree.root.repos
	project.SubProjects = tree.root.subProjects()
	return nil
}

// fetchDescendantGroups lists every group below groupID at any depth in one
// paginated walk.
func (c *gitLabProvider) fetchDescendantGroups(ctx context.Context, groupID string) ([]treeGroup, error) {
	groups := []treeGroup{}
	opt := &gitlab.ListDescendantGroupsOptions{ListOptions: gitLabListOptions}
	options := []gitlab.RequestOptionFunc{gitlab.WithContext(ctx)}
	what := fmt.Sprintf("GitLab subgroups from %s", groupID)
	err := paginate(ctx, c.log, what, constantPause(0), func(int) (bool, error) {
		gs, resp, err := c.client.Groups.ListDescendantGroups(groupID, opt, options...)
		if err != nil {
			return false, fmt.Errorf("unable to list subgroups: %w", err)
		}

		for _, g := range gs {
			groups = append(groups, treeGroup{
				id:       strconv.FormatInt(g.ID, 10),
				name:     g.Path,
				fullPath: g.FullPath,
			})
		}
		if resp.NextLink == "" {
			return false, nil
		}

		options = []gitlab.RequestOptionFunc{
			gitlab.WithContext(ctx),
			gitlab.WithKeysetPaginationParameters(resp.NextLink),
		}
		return true, nil
	})
	if err != nil {
		return nil, err
	}
	return groups, nil
}

func (c *gitLabProvider) fetchGroupProjects(ctx context.Context, groupID string) ([]domain.Repository, error) {
	projects := []domain.Repository{}
	includeSubGroups := true
	opt := &gitlab.ListGroupProjectsOptions{
		ListOptions:      gitLabListOptions,
		IncludeSubGroups: &includeSubGroups,
	}
	options := []gitlab.RequestOptionFunc{gitlab.WithContext(ctx)}
	what := fmt.Sprintf("GitLab projects from %s", groupID)
	err := paginate(ctx, c.log, what, constantPause(0), func(int) (bool, error) {
		ps, resp, err := c.client.Groups.ListGroupProjects(groupID, opt, options...)
		if err != nil {
			return false, fmt.Errorf("unable to list projects: %w", err)
		}

		for _, p := range ps {
			if skipGitLabProject(p, c.includeArchived) {
				continue
			}
			projects = append(projects, domain.Repository{
				ID:        strconv.FormatInt(p.ID, 10),
				Name:      p.Path,
				Namespace: p.Namespace.FullPath,
				Src:       p.SSHURLToRepo,
				URL:       p.WebURL,
				Desc:      p.Description,
			})
		}
		if resp.NextLink == "" {
			return false, nil
		}

		options = []gitlab.RequestOptionFunc{
			gitlab.WithContext(ctx),
			gitlab.WithKeysetPaginationParameters(resp.NextLink),
		}
		return true, nil
	})
	if err != nil {
		return nil, err
	}
	return projects, nil
}
