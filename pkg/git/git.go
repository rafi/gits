package git

import (
	"fmt"
	"os/exec"
)

type Git struct {
	bin string
}

func New(gitPath string) Git {
	return Git{
		bin: gitPath,
	}
}

// Exec executes git command-line with provided arguments.
func (g Git) Exec(path string, args []string) ([]byte, error) {
	var (
		cmdOut []byte
		err    error
	)
	args = append([]string{"-C", path}, args...)

	cmd := exec.Command(g.bin, args...)
	if cmdOut, err = cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("Failed to run %v\n%s %w", args, cmdOut, err)
	}
	return cmdOut, nil
}
