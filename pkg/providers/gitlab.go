package providers

import (
	"fmt"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/xanzy/go-gitlab"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/pkg/git"
)

var gitLabTokenEnvVarNames = []string{"GITLAB_TOKEN"}

type gitLabProvider struct {
	client     *gitlab.Client
	sourceType Provider
}

func newGitLabProvider(token string) (*gitLabProvider, error) {
	var err error
	provider := &gitLabProvider{sourceType: ProviderGitLab}
	if token == "" {
		token = findFirstEnvVar(gitLabTokenEnvVarNames)
	}
	if token == "" {
		return provider, fmt.Errorf("token is required for %s", provider.sourceType)
	}

	provider.client, err = gitlab.NewClient(token)
	if err != nil {
		return nil, fmt.Errorf("unable to create gitlab client: %w", err)
	}
	return provider, nil
}

var gitLabListOptions = gitlab.ListOptions{
	OrderBy:    "id",
	Pagination: "keyset",
	PerPage:    20,
	Sort:       "asc",
}

func (c *gitLabProvider) LoadRepos(groupID string, gitClient git.Git, project *domain.Project) error {
	var err error
	project.ID = groupID

	g, _, err := c.client.Groups.GetGroup(groupID, nil)
	if err != nil {
		return err
	}
	if project.Name == "" {
		project.Name = g.Name
	}
	project.SubProjects, err = c.fetchSubGroups(project.ID)
	if err != nil {
		return err
	}
	for i, group := range project.SubProjects {
		err := c.LoadRepos(group.ID, gitClient, &project.SubProjects[i])
		if err != nil {
			return err
		}
	}
	project.Repos, err = c.fetchGroupProjects(groupID)
	if err != nil {
		return err
	}
	return nil
}

func (c *gitLabProvider) fetchSubGroups(groupID string) ([]domain.Project, error) {
	groups := []domain.Project{}
	opt := &gitlab.ListSubGroupsOptions{ListOptions: gitLabListOptions}
	options := []gitlab.RequestOptionFunc{}
	pageNum := 0
	for {
		pageNum++
		log.Infof("Fetching gitlab subgroups from %s (%d)…", groupID, pageNum)

		gs, resp, err := c.client.Groups.ListSubGroups(groupID, opt, options...)
		if err != nil {
			return nil, fmt.Errorf("unable to list subgroups: %w", err)
		}

		for _, g := range gs {
			groups = append(groups, domain.Project{
				ID:   strconv.Itoa(g.ID),
				Name: g.Path,
			})
		}
		if resp.NextLink == "" {
			break
		}

		options = []gitlab.RequestOptionFunc{
			gitlab.WithKeysetPaginationParameters(resp.NextLink),
		}
	}

	return groups, nil
}

func (c *gitLabProvider) fetchGroupProjects(groupID string) ([]domain.Repository, error) {
	projects := []domain.Repository{}
	opt := &gitlab.ListGroupProjectsOptions{ListOptions: gitLabListOptions}
	options := []gitlab.RequestOptionFunc{}
	pageNum := 0
	for {
		pageNum++
		log.Infof("Fetching gitlab projects from %s (%d)…", groupID, pageNum)
		ps, resp, err := c.client.Groups.ListGroupProjects(groupID, opt, options...)
		if err != nil {
			return nil, fmt.Errorf("unable to list projects: %w", err)
		}

		// List all the projects we've found so far.
		for _, p := range ps {
			projects = append(projects, domain.Repository{
				ID:        strconv.Itoa(p.ID),
				Name:      p.Path,
				Namespace: strings.TrimPrefix(p.Namespace.FullPath, p.Path+"/"),
				Src:       p.SSHURLToRepo,
				URL:       p.WebURL,
				Desc:      p.Description,
			})
		}
		if resp.NextLink == "" {
			break
		}

		options = []gitlab.RequestOptionFunc{
			gitlab.WithKeysetPaginationParameters(resp.NextLink),
		}
	}

	return projects, nil
}
