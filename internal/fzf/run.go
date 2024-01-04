package fzf

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

const fzfBin = "fzf"

var (
	// sizeEnvVarNames are the names of the envvariables for width/height.
	sizeEnvVarNames = []string{"FZF_PREVIEW_COLUMNS", "FZF_PREVIEW_LINES"}

	defaultLayoutOpts = []string{
		"--height=50%",
		"--reverse",
	}

	// defaultOpts are the default options passed to fzf.
	defaultOpts = []string{
		"--ansi",
		"--info=right",
		"--no-multi",
		"--header-first",
		"--margin=1,3,0,3",
	}
)

// Run executes fzf with given args and stdin.
func Run(stdin bytes.Buffer, args ...string) (string, error) {
	_, err := exec.LookPath(fzfBin)
	if err != nil {
		return "", fmt.Errorf("%s not found in PATH", fzfBin)
	}

	// Default options
	args = append(args, defaultOpts...)
	if os.Getenv("FZF_DEFAULT_OPTS") == "" {
		args = append(args, defaultLayoutOpts...)
	}

	// Run shell command with stdin
	var cmdOut, cmdErr bytes.Buffer
	fzf := exec.Command(fzfBin, args...)
	fzf.Stdin = &stdin
	fzf.Stdout = &cmdOut
	fzf.Stderr = os.Stderr
	if err := fzf.Run(); err != nil {
		return "", err
	}
	if cmdErr.Len() > 0 {
		return "", fmt.Errorf("error: %s", cmdErr.String())
	}
	return strings.TrimSpace(cmdOut.String()), nil
}

// GetPreviewSize returns the preview size from fzf preview env variables.
func GetPreviewSize() (int, int, error) {
	sizes := []int{}
	for _, envName := range sizeEnvVarNames {
		value := os.Getenv(envName)
		if value != "" {
			size, err := strconv.Atoi(value)
			if err != nil {
				err = fmt.Errorf("unable to parse %s: %s", envName, value)
				return 0, 0, err
			}
			sizes = append(sizes, size)
		}
	}
	if len(sizes) < 2 {
		return 0, 0, nil
	}
	return sizes[0], sizes[1], nil
}
