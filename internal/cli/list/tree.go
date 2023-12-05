package list

import (
	"fmt"

	"github.com/xlab/treeprint"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cli"
)

// listTree lists projects and repos as a nested tree.
func listTree(projects domain.ProjectListKeyed, deps cli.RuntimeDeps) error {
	tree := makeTree(projects, deps)
	fmt.Println(tree.String())
	return nil
}

// makeTree builds a tree of a collection of projects.
func makeTree(projects domain.ProjectListKeyed, deps cli.RuntimeDeps) treeprint.Tree {
	tree := treeprint.New()
	for _, proj := range projects {
		branch := makeTreeProject(proj, deps)
		branch.SetValue(cli.ProjectTreeTitle(proj, deps.HomeDir, deps.Theme))
		if len(projects) == 1 {
			return branch
		}
		tree.AddBranch(branch)
	}

	return tree
}

// makeTreeProject recursively builds a tree of a single project.
func makeTreeProject(project domain.Project, deps cli.RuntimeDeps) treeprint.Tree {
	tree := treeprint.New()
	for _, subProj := range project.SubProjects {
		branch := makeTreeProject(subProj, deps)
		branch.SetValue(cli.ProjectTreeTitle(subProj, deps.HomeDir, deps.Theme))
		tree.AddBranch(branch)
	}
	for _, repo := range project.Repos {
		if project.AbsPath == "" {
			tree.AddNode(cli.Path(repo.AbsPath, deps.HomeDir))
		} else {
			tree.AddNode(repo.GetName())
		}
	}
	return tree
}
