package list

import (
	"fmt"

	log "github.com/sirupsen/logrus"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cli"
	"github.com/rafi/gits/internal/project"
)

func ExecList(format string, include []string, deps cli.RuntimeDeps) error {
	var lister func(domain.ProjectListKeyed, cli.RuntimeDeps) error
	switch format {
	case "json":
		lister = listJSON
	case "wide":
		lister = listWide
	case "table":
		lister = listTable
	case "tree":
		lister = listTree
	case "name":
		if len(include) > 0 {
			lister = listNameRepos
		} else {
			lister = listNameProjects
		}
	default:
		return fmt.Errorf("unknown output format: %s", format)
	}

	projects, err := project.GetProjects(include, deps)
	if err != nil {
		return fmt.Errorf("unable to list projects: %w", err)
	}
	if len(projects) == 0 {
		log.Warn("no projects found")
		return nil
	}
	return lister(projects, deps)
}
