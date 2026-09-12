package status

import (
	"fmt"
	"io"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cli/jsonout"
	"github.com/rafi/gits/internal/cli/walk"
)

// validateFormat rejects everything but the two formats `status` has.
// `list`'s other styles (name, tree, wide) are shapes `status` has no meaning
// for, and quietly falling back to the table would answer a question the user
// did not ask.
func validateFormat(format string) error {
	switch format {
	case "table", "json":
		return nil
	default:
		return fmt.Errorf("unknown output format %q, want table or json", format)
	}
}

// renderJSON writes the collected results as the envelope `list -o json`
// shares, with each probed repository's working-tree data nested under it.
// With tree set, groups are the walker's depth-first traversal of a whole
// project; without it they are one repository under its project.
func renderJSON(w io.Writer, groups []walk.GroupResult, tree bool, opts Options) error {
	env := jsonout.Envelope{}
	if len(groups) == 0 {
		return jsonout.Write(w, env)
	}

	var (
		node jsonout.Project
		keep bool
	)
	if tree {
		node, keep, _ = buildNode(groups, 0, opts)
	} else {
		node, keep = buildLeaf(groups[0], opts)
	}
	if keep {
		env[groups[0].Project.Name] = node
	}
	return jsonout.Write(w, env)
}

// buildNode rebuilds one project subtree from the flat groups, inverting the
// walker's depth-first order: a group is followed by one subtree per
// sub-project of its project. It returns the node, whether it survived the
// active filters, and the index of the next unconsumed group.
func buildNode(groups []walk.GroupResult, i int, opts Options) (jsonout.Project, bool, int) {
	if i >= len(groups) {
		return jsonout.Project{}, false, i
	}
	g := groups[i]
	node := jsonout.NewProject(g.Project)
	node.Repos = buildRepos(g, opts)

	next := i + 1
	for range g.Project.SubProjects {
		var (
			sub  jsonout.Project
			keep bool
		)
		sub, keep, next = buildNode(groups, next, opts)
		if keep {
			node.SubProjects = append(node.SubProjects, sub)
		}
	}
	return node, keptNode(node, opts), next
}

// buildLeaf converts a single group without descending, for the single-repo
// form where the group's project carries sub-projects the walker never
// visited.
func buildLeaf(g walk.GroupResult, opts Options) (jsonout.Project, bool) {
	node := jsonout.NewProject(g.Project)
	node.Repos = buildRepos(g, opts)
	return node, keptNode(node, opts)
}

// keptNode reports whether a project node appears in the document. A filtered
// run drops nodes left with nothing under them, as the table drops a project
// whose every row was hidden.
//
// One divergence the tree forces: a project with no surviving repositories of
// its own stays when a sub-project survived, because that sub-project has
// nowhere else to hang. The table has no such constraint — its groups are
// flat, so it judges each one alone and the emptied parent's header just
// goes. The node that stays omits `repos` entirely, as any project with none
// does.
func keptNode(node jsonout.Project, opts Options) bool {
	if !opts.filtered() {
		return true
	}
	return len(node.Repos) > 0 || len(node.SubProjects) > 0
}

// buildRepos converts one group's visible statuses, so the document holds
// exactly the repositories the table would have shown rows for.
func buildRepos(g walk.GroupResult, opts Options) []jsonout.Repository {
	sts, _ := visibleStatuses(g, opts)
	repos := make([]jsonout.Repository, 0, len(sts))
	for _, st := range sts {
		repos = append(repos, buildRepo(st))
	}
	if len(repos) == 0 {
		// A project with no repositories omits the key rather than
		// shipping an empty list.
		return nil
	}
	return repos
}

// buildRepo converts one repository's identity, state and — where git was
// actually consulted — its working tree.
func buildRepo(st *repoStatus) jsonout.Repository {
	repo := jsonout.NewRepository(st.repo)
	if st.repo.State != domain.RepoStateOK {
		// Nothing was probed: the state and its reason are the whole story.
		return repo
	}
	if st.err != nil {
		repo.Status = &jsonout.Status{Error: st.message}
		return repo
	}

	status := &jsonout.Status{
		Branch:    st.branch,
		Staged:    st.staged,
		Unstaged:  st.unstaged,
		Untracked: st.untracked,
		Ahead:     st.ahead,
		Behind:    st.behind,
		Compared:  !st.noUpstream,
		Version:   st.version,
	}
	// hasStat is the whole condition: the probe only runs under --stat, so a
	// measured diff is already a --stat diff.
	if st.hasStat {
		status.Head = &jsonout.Head{Added: st.added, Deleted: st.deleted}
	}
	if st.commit != "" {
		status.Commit = &jsonout.Commit{
			Hash:    st.commit,
			Subject: st.message,
			Time:    st.when,
		}
	}
	repo.Status = status
	return repo
}
