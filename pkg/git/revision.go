package git

import (
	"fmt"
	"strings"
)

// CurrentPosition returns a short log description of HEAD
func (g Git) CurrentPosition(path string) (string, error) {
	args := []string{"log", "-1", "--color=always", "--format=%C(auto)%D %C(242)(%aN %ar)%Creset"}
	output, err := g.Exec(path, args)
	if err != nil {
		return "", fmt.Errorf("Unable to get current rev: %w", err)
	}
	return strings.TrimSuffix(string(output), "\n"), nil
}

// Describe generates a version description based on tags and hash
func (g Git) Describe(path string) (string, error) {
	args := []string{"describe", "--always"}
	desc, err := g.Exec(path, args)
	if err != nil {
		return "", fmt.Errorf("Unable to describe rev: %w", err)
	}
	return strings.TrimSuffix(string(desc), "\n"), nil
}
