package list

import (
	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/app"
	"github.com/rafi/gits/internal/runtime/json"
)

// listJSON outputs projects as the shared JSON envelope, without the nested
// working-tree data only `status` gathers.
func listJSON(projects domain.ProjectListKeyed, deps app.RuntimeCLI) error {
	return json.Write(deps.Out, json.FromProjects(projects))
}
