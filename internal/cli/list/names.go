package list

import (
	"fmt"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/types"
)

func listNameProjects(projects domain.ProjectListKeyed, _ types.RuntimeCLI) error {
	for _, name := range projects.SortedNames() {
		fmt.Println(name)
	}
	return nil
}

func listNameRepos(projects domain.ProjectListKeyed, _ types.RuntimeCLI) error {
	for _, projName := range projects.SortedNames() {
		proj := projects[projName]
		for _, name := range proj.ListReposWithNamespace() {
			fmt.Println(name)
		}
	}
	return nil
}
