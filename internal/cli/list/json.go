package list

import (
	"encoding/json"
	"fmt"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cli"
)

func listJSON(projects domain.ProjectListKeyed, _ cli.RuntimeDeps) error {
	raw, err := json.Marshal(projects)
	if err != nil {
		return fmt.Errorf("unable to marshal json: %w", err)
	}
	fmt.Println(string(raw))
	return nil
}
