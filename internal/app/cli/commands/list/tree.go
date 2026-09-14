package list

import (
	"charm.land/lipgloss/v2"
	"github.com/xlab/treeprint"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/app"
	"github.com/rafi/gits/internal/app/cli/render/style"
	"github.com/rafi/gits/internal/format"
)

// listTree lists projects and repos as a nested tree.
func listTree(projects domain.ProjectListKeyed, deps app.RuntimeCLI) error {
	tree := makeTree(projects, deps)
	lipgloss.Fprint(deps.Out, tree.String())
	return nil
}

// makeTree builds a tree of a collection of projects. Projects are walked in
// SortedNames order so two runs render the same tree.
func makeTree(projects domain.ProjectListKeyed, deps app.RuntimeCLI) treeprint.Tree {
	tree := treeprint.New()
	for _, name := range projects.SortedNames() {
		proj := projects[name]
		branch := makeTreeProject(proj, deps)
		branch.SetValue(style.ProjectTreeTitle(proj, deps.HomeDir, deps.Theme))
		if len(projects) == 1 {
			return branch
		}
		tree.AddBranch(branch)
	}

	return tree
}

// makeTreeProject recursively builds a tree of a single project.
func makeTreeProject(project domain.Project, deps app.RuntimeCLI) treeprint.Tree {
	tree := treeprint.New()
	for _, subProj := range project.SubProjects {
		branch := makeTreeProject(subProj, deps)
		branch.SetValue(style.ProjectTreeTitle(subProj, deps.HomeDir, deps.Theme))
		tree.AddBranch(branch)
	}
	for _, repo := range project.Repos {
		if project.AbsPath == "" {
			tree.AddNode(format.Path(repo.AbsPath, deps.HomeDir))
		} else {
			tree.AddNode(repo.GetName())
		}
	}
	return tree
}
