package list

import (
	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cli/jsonout"
	"github.com/rafi/gits/internal/types"
)

// listJSON outputs projects as the shared JSON envelope, without the nested
// working-tree data only `status` gathers.
func listJSON(projects domain.ProjectListKeyed, deps types.RuntimeCLI) error {
	return jsonout.Write(deps.Out, jsonout.FromProjects(projects))
}
