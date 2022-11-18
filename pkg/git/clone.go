package git

import (
	"fmt"
	"os"
	"os/exec"
)

// Clone clones repository, if not cloned already
func (g Git) Clone(remote string, path string) (string, error) {
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		return "", fmt.Errorf("Directory already exists")
	}

	args := []string{"clone", remote, path}
	cmd := exec.Command("git", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("Unable to clone: %w", err)
	}
	return string(output), nil
}
