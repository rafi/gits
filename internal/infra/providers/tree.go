package providers

import (
	"sort"
	"strings"

	"github.com/rafi/gits/domain"
)

// treeGroup is a group of repositories, reduced to what the tree needs: a
// provider group, or a directory of repository paths.
type treeGroup struct {
	id       string
	name     string
	fullPath string
}

// treeNode is one node of the group tree while it is being assembled;
// domain.Project stores children by value, which a partially built tree
// cannot.
type treeNode struct {
	id       string
	name     string
	children []*treeNode
	repos    []domain.Repository
}

// subProjects converts the node's children into domain Sub-projects,
// depth-first, preserving the order they were added in.
func (n *treeNode) subProjects() []domain.Project {
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

// repoTree assembles a Sub-project tree from one flat list of groups and
// one flat list of repositories, keyed by their full `/`-separated paths.
type repoTree struct {
	rootPath string
	root     *treeNode
	byPath   map[string]*treeNode
}

func newRepoTree(rootPath string) *repoTree {
	root := &treeNode{repos: []domain.Repository{}}
	return &repoTree{
		rootPath: rootPath,
		root:     root,
		byPath:   map[string]*treeNode{rootPath: root},
	}
}

// addGroups attaches every descendant group to its parent. Groups are sorted
// by depth first so a parent always exists before its children, while
// siblings keep the order the API returned them in.
func (t *repoTree) addGroups(groups []treeGroup) {
	sorted := make([]treeGroup, len(groups))
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
		node := &treeNode{id: g.id, name: g.name, repos: []domain.Repository{}}
		parent.children = append(parent.children, node)
		t.byPath[g.fullPath] = node
	}
}

// addRepos files each repository under the group named by its namespace,
// falling back to the root group when that namespace is unknown.
func (t *repoTree) addRepos(repos []domain.Repository) {
	for _, repo := range repos {
		node, ok := t.byPath[repo.Namespace]
		if !ok {
			node = t.root
		}
		node.repos = append(node.repos, repo)
	}
}
