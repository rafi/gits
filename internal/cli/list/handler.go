package list

import (
	"fmt"

	log "github.com/sirupsen/logrus"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/loader"
	"github.com/rafi/gits/internal/types"
)

// ExecList displays a list of projects and repositories.
//
// Args: (optional)
//   - project names
func ExecList(format string, args []string, deps types.RuntimeCLI) error {
	var lister func(domain.ProjectListKeyed, types.RuntimeCLI) error
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
		if len(args) > 0 {
			lister = listNameRepos
		} else {
			lister = listNameProjects
		}
	default:
		return fmt.Errorf("unknown output format %q", format)
	}

	projects, err := loader.GetProjects(args, deps.Runtime)
	if err != nil {
		return err
	}
	if len(projects) == 0 {
		log.Warn("no projects found")
		return nil
	}
	return lister(projects, deps)
}
