package providers

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"

	gitlab "gitlab.com/gitlab-org/api/client-go"

	"github.com/rafi/gits/domain"
)

type gitLabProvider struct {
	client          *gitlab.Client
	log             *slog.Logger
	includeArchived bool
}

func newGitLabProvider(opts Options) (*gitLabProvider, error) {
	clientOpts := []gitlab.ClientOptionFunc{}
	if opts.Timeout > 0 {
		clientOpts = append(clientOpts,
			gitlab.WithHTTPClient(&http.Client{Timeout: opts.Timeout}))
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

	tree := newGitLabTree(root.FullPath)
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

// gitLabGroup is a descendant group reduced to what the tree needs.
type gitLabGroup struct {
	id       string
	name     string
	fullPath string
}

// gitLabNode is one node of the group tree while it is being assembled;
// domain.Project stores children by value, which a partially built tree
// cannot.
type gitLabNode struct {
	id       string
	name     string
	children []*gitLabNode
	repos    []domain.Repository
}

// subProjects converts the node's children into domain Sub-projects,
// depth-first, preserving the order they were added in.
func (n *gitLabNode) subProjects() []domain.Project {
	if len(n.children) == 0 {
		return nil
	}
	projects := make([]domain.Project, 0, len(n.children))
	for _, child := range n.children {
		projects = append(projects, domain.Project{
			ID:          child.id,
			Name:        child.name,
			Repos:       child.repos,
			SubProjects: child.subProjects(),
		})
	}
	return projects
}

// gitLabTree assembles a group tree from one flat list of descendant groups
// and one flat list of projects, keyed by their GitLab full paths.
type gitLabTree struct {
	rootPath string
	root     *gitLabNode
	byPath   map[string]*gitLabNode
}

func newGitLabTree(rootPath string) *gitLabTree {
	root := &gitLabNode{repos: []domain.Repository{}}
	return &gitLabTree{
		rootPath: rootPath,
		root:     root,
		byPath:   map[string]*gitLabNode{rootPath: root},
	}
}

// addGroups attaches every descendant group to its parent. Groups are sorted
// by depth first so a parent always exists before its children, while
// siblings keep the order the API returned them in.
func (t *gitLabTree) addGroups(groups []gitLabGroup) {
	sorted := make([]gitLabGroup, len(groups))
	copy(sorted, groups)
	sort.SliceStable(sorted, func(i, j int) bool {
		return strings.Count(sorted[i].fullPath, "/") <
			strings.Count(sorted[j].fullPath, "/")
	})

	for _, g := range sorted {
		parentPath := g.fullPath[:max(strings.LastIndex(g.fullPath, "/"), 0)]
		parent, ok := t.byPath[parentPath]
		if !ok {
			// A group whose parent is outside the tree, or is invisible to
			// this token: hang it off the root rather than dropping it.
			parent = t.root
		}
		node := &gitLabNode{id: g.id, name: g.name, repos: []domain.Repository{}}
		parent.children = append(parent.children, node)
		t.byPath[g.fullPath] = node
	}
}

// addRepos files each repository under the group named by its namespace,
// falling back to the root group when that namespace is unknown.
func (t *gitLabTree) addRepos(repos []domain.Repository) {
	for _, repo := range repos {
		node, ok := t.byPath[repo.Namespace]
		if !ok {
			node = t.root
		}
		node.repos = append(node.repos, repo)
	}
}

// fetchDescendantGroups lists every group below groupID at any depth in one
// paginated walk.
func (c *gitLabProvider) fetchDescendantGroups(ctx context.Context, groupID string) ([]gitLabGroup, error) {
	groups := []gitLabGroup{}
	opt := &gitlab.ListDescendantGroupsOptions{ListOptions: gitLabListOptions}
	options := []gitlab.RequestOptionFunc{gitlab.WithContext(ctx)}
	what := fmt.Sprintf("GitLab subgroups from %s", groupID)
	err := paginate(ctx, c.log, what, constantPause(0), func(int) (bool, error) {
		gs, resp, err := c.client.Groups.ListDescendantGroups(groupID, opt, options...)
		if err != nil {
			return false, fmt.Errorf("unable to list subgroups: %w", err)
		}

		for _, g := range gs {
			groups = append(groups, gitLabGroup{
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
