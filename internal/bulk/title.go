package bulk

import (
	"charm.land/lipgloss/v2"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cli"
	"github.com/rafi/gits/internal/types"
)

// titleWidths holds the widest repository title of every project node in a
// tree, measured once up front so the module does not remeasure a whole
// project for each of its repositories. Nodes are identified by their Repos
// backing array, the one part of a project value that stays shared as project
// nodes are copied around.
type titleWidths map[*domain.Repository]int

// newTitleWidths measures every project node of root, root itself included,
// matching the traversal so each group keeps its own padding.
func newTitleWidths(root domain.Project, homeDir string) titleWidths {
	widths := titleWidths{}
	var visit func(p domain.Project)
	visit = func(p domain.Project) {
		if len(p.Repos) > 0 {
			widths[&p.Repos[0]] = maxTitleWidth(p, homeDir)
		}
		for _, sub := range p.SubProjects {
			visit(sub)
		}
	}
	visit(root)
	return widths
}

// For returns project's precomputed title width; a project without
// repositories has no titles to pad and yields zero.
func (tw titleWidths) For(project domain.Project) int {
	if len(project.Repos) == 0 {
		return 0
	}
	return tw[&project.Repos[0]]
}

// maxTitleWidth returns the rendered width of the widest repository display
// path in a project, measured on the same derivation the titles render.
func maxTitleWidth(project domain.Project, homeDir string) int {
	maxLen := 0
	for _, repo := range project.Repos {
		maxLen = max(maxLen, lipgloss.Width(cli.RepoRelPath(project, repo, homeDir)))
	}
	return maxLen
}

// paddedRepoTitle is the title every body receives: the repository's display
// path padded to width — its project's widest, precomputed once per run — so
// the result bodies align.
func paddedRepoTitle(
	repo domain.Repository,
	project domain.Project,
	width int,
	deps types.RuntimeCLI,
) lipgloss.Style {
	return cli.RepoTitle(repo, project, deps.HomeDir, deps.Theme).Width(width)
}
