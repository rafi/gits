package git

import (
	"fmt"
	"strings"
)

// CurrentBranch returns current branch
func (g Git) CurrentBranch(path string) (string, error) {
	args := []string{"rev-parse", "--abbrev-ref", "HEAD"}
	output, err := g.Exec(path, args)
	if err != nil {
		return "", fmt.Errorf("Unable to detect current branch: %w", err)
	}
	branch := strings.TrimSuffix(string(output), "\n")
	return branch, nil
}

// Branches returns list of branches, local and remote
func (g Git) Branches(path string) ([]string, error) {
	args := []string{"for-each-ref", "--shell", "--format=%(refname)", "refs"}
	output, err := g.Exec(path, args)
	if err != nil {
		return nil, fmt.Errorf("Unable to list branches: %w", err)
	}
	refs := strings.Split(strings.TrimSuffix(string(output), "\n"), "\n")
	branches := []string{}
	for _, ref := range refs {
		ref = strings.Trim(ref, "'")
		parts := strings.Split(ref, "/")
		if parts[len(parts)-1] != "HEAD" {
			ref := strings.Join(parts[2:], "/")
			if len(ref) > 0 {
				branches = append(branches, ref)
			}
		}
	}
	return branches, nil
}
