package list

import (
	"fmt"
	"path/filepath"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/types"
)

func listNameProjects(projects domain.ProjectListKeyed, _ types.RuntimeCLI) error {
	for _, proj := range projects {
		fmt.Println(proj.Name)
	}
	return nil
}

func listNameRepos(projects domain.ProjectListKeyed, _ types.RuntimeCLI) error {
	for _, proj := range projects {
		makeName(proj)
	}
	return nil
}

func makeName(project domain.Project) {
	for _, repo := range project.Repos {
		fmt.Println(repo.GetName())
	}
	for projIdx := range project.SubProjects {
		proj := project.SubProjects[projIdx]
		for repoIdx, repo := range proj.Repos {
			proj.Repos[repoIdx].Name = filepath.Join(proj.Name, repo.Name)
		}
		makeName(proj)
	}
}
