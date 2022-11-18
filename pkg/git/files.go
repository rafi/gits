package git

import (
	"fmt"
	"strings"
)

// Untracked returns the number of untracked files
func (g Git) Untracked(path string) (int, error) {
	args := []string{"ls-files", "--others", "--exclude-standard"}
	output, err := g.Exec(path, args)
	if err != nil {
		return 0, fmt.Errorf("Unable to find untracked: %w", err)
	}
	return len(strings.Split(string(output), "\n")) - 1, nil
}
