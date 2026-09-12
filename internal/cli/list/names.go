package list

import (
	"fmt"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/types"
)

func listNameProjects(projects domain.ProjectListKeyed, deps types.RuntimeCLI) error {
	for _, name := range projects.SortedNames() {
		fmt.Fprintln(deps.Out, name)
	}
	return nil
}

func listNameRepos(projects domain.ProjectListKeyed, deps types.RuntimeCLI) error {
	for _, projName := range projects.SortedNames() {
		proj := projects[projName]
		for _, name := range proj.ListReposWithNamespace() {
			fmt.Fprintln(deps.Out, name)
		}
	}
	return nil
}
