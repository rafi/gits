package providers

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	gitlab "gitlab.com/gitlab-org/api/client-go"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/pkg/git"
)

var gitLabTokenEnvVarNames = []string{"GITLAB_TOKEN"}

type gitLabProvider struct {
	client     *gitlab.Client
	sourceType Provider
}

func newGitLabProvider(opts Options) (*gitLabProvider, error) {
	token := opts.Token
	var err error
	provider := &gitLabProvider{sourceType: ProviderGitLab}
	if token == "" {
		token = getFirstEnvValue(gitLabTokenEnvVarNames)
	}
	if token == "" {
		return provider, fmt.Errorf("token is required for %s", provider.sourceType)
	}

	clientOpts := []gitlab.ClientOptionFunc{}
	if opts.Timeout > 0 {
		clientOpts = append(clientOpts,
			gitlab.WithHTTPClient(&http.Client{Timeout: opts.Timeout}))
	}
	provider.client, err = gitlab.NewClient(token, clientOpts...)
	if err != nil {
		return nil, fmt.Errorf("unable to create gitlab client: %w", err)
	}
	return provider, nil
}

var gitLabListOptions = gitlab.ListOptions{
	OrderBy:    "id",
	Pagination: "keyset",
	PerPage:    50,
	Sort:       "asc",
}

func (c *gitLabProvider) LoadRepos(ctx context.Context, groupID string, gitClient git.GitClient, project *domain.Project) error {
	var err error
	project.ID = groupID

	g, _, err := c.client.Groups.GetGroup(groupID, nil, gitlab.WithContext(ctx))
	if err != nil {
		return err
	}
	if project.Name == "" {
		project.Name = g.Name
	}
	project.SubProjects, err = c.fetchSubGroups(ctx, project.ID)
	if err != nil {
		return err
	}
	for i, group := range project.SubProjects {
		err := c.LoadRepos(ctx, group.ID, gitClient, &project.SubProjects[i])
		if err != nil {
			return err
		}
	}
	project.Repos, err = c.fetchGroupProjects(ctx, groupID)
	if err != nil {
		return err
	}
	return nil
}

func (c *gitLabProvider) fetchSubGroups(ctx context.Context, groupID string) ([]domain.Project, error) {
	groups := []domain.Project{}
	opt := &gitlab.ListSubGroupsOptions{ListOptions: gitLabListOptions}
	options := []gitlab.RequestOptionFunc{gitlab.WithContext(ctx)}
	what := fmt.Sprintf("GitLab subgroups from %s", groupID)
	err := paginate(ctx, what, 0, func(int) (bool, error) {
		gs, resp, err := c.client.Groups.ListSubGroups(groupID, opt, options...)
		if err != nil {
			return false, fmt.Errorf("unable to list subgroups: %w", err)
		}

		for _, g := range gs {
			groups = append(groups, domain.Project{
				ID:   strconv.FormatInt(g.ID, 10),
				Name: g.Path,
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
	opt := &gitlab.ListGroupProjectsOptions{ListOptions: gitLabListOptions}
	options := []gitlab.RequestOptionFunc{gitlab.WithContext(ctx)}
	what := fmt.Sprintf("GitLab projects from %s", groupID)
	err := paginate(ctx, what, 0, func(int) (bool, error) {
		ps, resp, err := c.client.Groups.ListGroupProjects(groupID, opt, options...)
		if err != nil {
			return false, fmt.Errorf("unable to list projects: %w", err)
		}

		for _, p := range ps {
			if p.Archived || p.EmptyRepo {
				continue
			}
			projects = append(projects, domain.Repository{
				ID:        strconv.FormatInt(p.ID, 10),
				Name:      p.Path,
				Namespace: strings.TrimPrefix(p.Namespace.FullPath, p.Path+"/"),
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
