package status

import (
	"io"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/bulk"
	"github.com/rafi/gits/internal/cli/jsonout"
)

// renderJSON writes the run's results as the envelope `list -o json` shares,
// with each probed repository's working-tree data nested under it. The
// document's tree is the one the run visited — a whole project, or one
// repository under its project.
func renderJSON(w io.Writer, res bulk.Results[*repoStatus], opts Options) error {
	env := jsonout.Envelope{}
	if res.Project.Name == "" {
		// The named project was skipped: nothing ran, nothing to document.
		return jsonout.Write(w, env)
	}
	if node, keep := buildNode(res.Project, newRows(res), opts); keep {
		env[res.Project.Name] = node
	}
	return jsonout.Write(w, env)
}

// buildNode converts one project subtree, returning the node and whether it
// survived the active filters. A filtered run drops nodes left with nothing
// under them, as the table drops a project whose every row was hidden.
//
// One divergence the tree forces: a project with no surviving repositories of
// its own stays when a sub-project survived, because that sub-project has
// nowhere else to hang. The table has no such constraint — it judges each
// project alone and the emptied parent's table just goes. The node that stays
// omits `repos` entirely, as any project with none does.
func buildNode(p domain.Project, index rows, opts Options) (jsonout.Project, bool) {
	node := jsonout.NewProject(p)
	sts, _ := index.visible(p, opts)
	for _, st := range sts {
		node.Repos = append(node.Repos, buildRepo(st))
	}
	for _, sub := range p.SubProjects {
		if child, keep := buildNode(sub, index, opts); keep {
			node.SubProjects = append(node.SubProjects, child)
		}
	}
	if !opts.filtered() {
		return node, true
	}
	return node, len(node.Repos) > 0 || len(node.SubProjects) > 0
}

// buildRepo converts one repository's identity, state and — where git was
// actually consulted — its working tree.
func buildRepo(st *repoStatus) jsonout.Repository {
	repo := jsonout.NewRepository(st.repo.Repository)
	if st.repo.State != domain.RepoStateOK {
		// Nothing was probed: the state and its reason are the whole story.
		return repo
	}
	if st.err != nil {
		repo.Status = &jsonout.Status{Error: st.err.Error()}
		return repo
	}

	status := &jsonout.Status{
		Branch:    st.Branch,
		Staged:    st.Staged,
		Unstaged:  st.Unstaged,
		Untracked: st.Untracked,
		Ahead:     st.Ahead,
		Behind:    st.Behind,
		Compared:  st.compared,
		Version:   st.version,
	}
	// The object's absence is what says no Upstream is configured, so a branch
	// that was never pushed is structurally distinct from one whose Upstream
	// went away.
	if st.Upstream != "" {
		status.Upstream = &jsonout.Upstream{
			Name:    st.Upstream,
			Tracked: !st.GoneUpstream(),
		}
	}
	if st.stat != nil {
		status.Head = &jsonout.Head{Added: st.stat.Added, Deleted: st.stat.Deleted}
	}
	if st.head.Hash != "" {
		status.Commit = &jsonout.Commit{
			Hash:    st.head.Hash,
			Subject: st.head.Subject,
			Time:    st.head.Time,
		}
	}
	repo.Status = status
	return repo
}
