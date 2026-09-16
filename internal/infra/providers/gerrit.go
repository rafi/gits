package providers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"slices"
	"strconv"
	"strings"

	gerrit "github.com/andygrunwald/go-gerrit"

	"github.com/rafi/gits/domain"
)

// gerritPageSize is how many projects one listing page asks for.
const gerritPageSize = 100

// gerritProjectPlaceholder stands for the project name in a download scheme
// URL.
const gerritProjectPlaceholder = "${project}"

// gerritMetaProjects hold server configuration, not code, and are never
// listed.
var gerritMetaProjects = []string{"All-Projects", "All-Users"}

// gerritProvider discovers the projects of a Gerrit host by name prefix.
type gerritProvider struct {
	client          *gerrit.Client
	baseURL         string
	username        string
	log             *slog.Logger
	includeArchived bool
}

// newGerritProvider builds a client for opts.BaseURL, anonymous unless a
// token resolved. The token is an HTTP password, so it needs a username;
// credentials never go into the client URL, which would make go-gerrit probe
// them with extra requests.
func newGerritProvider(opts Options) (*gerritProvider, error) {
	if opts.BaseURL == "" {
		return nil, errors.New("url is required for gerrit")
	}
	if opts.Token != "" && opts.Username == "" {
		return nil, fmt.Errorf(
			"gerrit at %s: a token needs a username: set username in the source, its url, or settings.gerrit",
			opts.BaseURL)
	}
	// Without credentials in the URL, NewClient sends no request.
	client, err := gerrit.NewClient(context.Background(), opts.BaseURL, opts.httpClient(domain.ProviderGerrit))
	if err != nil {
		return nil, fmt.Errorf("unable to create gerrit client: %w", err)
	}
	if opts.Token != "" {
		client.Authentication.SetBasicAuth(opts.Username, opts.Token)
	}
	return &gerritProvider{
		client:          client,
		baseURL:         opts.BaseURL,
		username:        opts.Username,
		log:             opts.Log,
		includeArchived: opts.IncludeArchived,
	}, nil
}

// LoadRepos lists the projects whose names start with prefix. Their `/`
// segments become Sub-projects, rooted at the last complete segment of
// prefix: `openstack/` puts openstack/nova at the root as nova.
func (c *gerritProvider) LoadRepos(ctx context.Context, prefix string, project *domain.Project) error {
	project.ID = prefix

	cloneURL, err := c.cloneURLFunc(ctx)
	if err != nil {
		return err
	}
	infos, err := c.fetchProjects(ctx, prefix)
	if err != nil {
		return err
	}

	names := make([]string, 0, len(infos))
	for name, info := range infos {
		if !c.skipProject(name, info) {
			names = append(names, name)
		}
	}
	slices.Sort(names)

	rootPath := prefix[:max(strings.LastIndex(prefix, "/"), 0)]
	tree := newRepoTree(rootPath)
	tree.addGroups(gerritDirectories(rootPath, names))
	repos := make([]domain.Repository, 0, len(names))
	for _, name := range names {
		slash := strings.LastIndex(name, "/")
		repos = append(repos, domain.Repository{
			ID:        name,
			Name:      name[slash+1:],
			Namespace: name[:max(slash, 0)],
			Src:       cloneURL(name),
			URL:       c.baseURL + "/admin/repos/" + name,
			Desc:      infos[name].Description,
		})
	}
	tree.addRepos(repos)

	project.Repos = tree.root.repos
	project.SubProjects = tree.root.subProjects()
	return nil
}

// fetchProjects walks every page of the prefix listing, keyed by name. Gerrit
// marks the last entry of a page with `_more_projects` when another follows.
func (c *gerritProvider) fetchProjects(ctx context.Context, prefix string) (map[string]gerrit.ProjectInfo, error) {
	infos := map[string]gerrit.ProjectInfo{}
	what := fmt.Sprintf("gerrit projects with prefix %q", prefix)
	err := paginate(ctx, c.log, what, constantPause(0), func(int) (bool, error) {
		opt := &gerrit.ProjectOptions{Prefix: prefix, Description: true}
		opt.Limit = gerritPageSize
		if len(infos) > 0 {
			opt.Skip = strconv.Itoa(len(infos))
		}
		page, _, err := c.client.Projects.ListProjects(ctx, opt)
		if err != nil {
			return false, fmt.Errorf("gerrit: list projects with prefix %q: %w", prefix, err)
		}
		more := false
		for name, info := range *page {
			more = more || info.MoreProjects
			infos[name] = info
		}
		return more && len(*page) > 0, nil
	})
	if err != nil {
		return nil, err
	}
	return infos, nil
}

// skipProject reports whether a listed project is omitted: meta and hidden
// ones always, read-only ones unless settings.includeArchived is set.
func (c *gerritProvider) skipProject(name string, info gerrit.ProjectInfo) bool {
	switch {
	case slices.Contains(gerritMetaProjects, name), info.State == "HIDDEN":
		return true
	case info.State == "READ_ONLY":
		return !c.includeArchived
	}
	return false
}

// gerritDirectories returns every directory of names below rootPath as a
// tree group, parents before children.
func gerritDirectories(rootPath string, names []string) []treeGroup {
	seen := map[string]bool{}
	groups := []treeGroup{}
	for _, name := range names {
		for i, r := range name {
			if r != '/' || i <= len(rootPath) {
				continue
			}
			dir := name[:i]
			if seen[dir] {
				continue
			}
			seen[dir] = true
			groups = append(groups, treeGroup{
				id:       dir,
				name:     dir[strings.LastIndex(dir, "/")+1:],
				fullPath: dir,
			})
		}
	}
	return groups
}

// cloneURLFunc asks the server once how it is cloned from and returns the
// clone URL of a project name: SSH at the advertised host and port with the
// username, else the advertised HTTP URL, else the web host itself.
func (c *gerritProvider) cloneURLFunc(ctx context.Context) (func(name string) string, error) {
	info, _, err := c.client.Config.GetServerInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("gerrit: get server info: %w", err)
	}
	schemes := info.Download.Schemes

	if ssh, ok := schemes["ssh"]; ok {
		u, err := url.Parse(strings.TrimSuffix(ssh.URL, gerritProjectPlaceholder))
		if err == nil && u.Host != "" {
			base := "ssh://" + u.Host + "/"
			if c.username != "" {
				base = "ssh://" + url.User(c.username).String() + "@" + u.Host + "/"
			}
			return func(name string) string { return base + name }, nil
		}
	}
	for _, scheme := range []string{"http", "anonymous http"} {
		if http, ok := schemes[scheme]; ok && strings.Contains(http.URL, gerritProjectPlaceholder) {
			return func(name string) string {
				return strings.ReplaceAll(http.URL, gerritProjectPlaceholder, name)
			}, nil
		}
	}
	return func(name string) string { return c.baseURL + "/" + name }, nil
}
