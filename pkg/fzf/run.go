package fzf

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

const fzfBin = "fzf"

var (
	// ErrAborted is returned when the user cancels the finder (Esc/Ctrl-C).
	ErrAborted = errors.New("selection aborted")
	// ErrNoMatch is returned when fzf exits without a match.
	ErrNoMatch = errors.New("no match")
)

type FZF struct {
	Args []string
}

var (
	// defaultOpts are the default options passed to fzf.
	defaultOpts = []string{
		"--ansi",
		"--info=right",
		"--no-multi",
		"--header-first",
		"--margin=1,3,0,3",
	}

	// Unless user has set FZF_DEFAULT_OPTS, we set some sane defaults.
	defaultLayoutOpts = []string{
		"--height=50%",
		"--reverse",
	}

	// defaultPreviewOpts are the default options for preview window.
	defaultPreviewOpts = "right,70%"

	// sizeEnvVarNames are the names of the envvariables for width/height.
	sizeEnvVarNames = []string{"FZF_PREVIEW_COLUMNS", "FZF_PREVIEW_LINES"}
)

func New(args ...string) *FZF {
	return &FZF{Args: args}
}

func (f *FZF) WithPreview(cmd, opts string) {
	f.Args = append(f.Args, "--preview", cmd)
	if opts == "" {
		opts = defaultPreviewOpts
	}
	f.Args = append(f.Args, "--preview-window", opts)
}

func (f *FZF) WithPrompt(label string) {
	f.Args = append(f.Args, "--prompt", label)
}

// Run executes fzf with given args and stdin. Cancelling by the user maps to
// ErrAborted, and an empty match to ErrNoMatch, so callers can exit quietly.
func (f *FZF) Run(ctx context.Context, stdin bytes.Buffer) (string, error) {
	_, err := exec.LookPath(fzfBin)
	if err != nil {
		return "", fmt.Errorf("%s not found in PATH", fzfBin)
	}

	// Defaults first so any conflicting caller option wins (fzf is
	// last-flag-wins); copying also avoids aliasing the caller's slice.
	args := append([]string{}, defaultOpts...)
	if os.Getenv("FZF_DEFAULT_OPTS") == "" {
		args = append(args, defaultLayoutOpts...)
	}
	args = append(args, f.Args...)

	// Run shell command with stdin
	var cmdOut bytes.Buffer
	fzf := exec.CommandContext(ctx, fzfBin, args...)
	fzf.Stdin = &stdin
	fzf.Stdout = &cmdOut
	fzf.Stderr = os.Stderr
	if err := fzf.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			switch exitErr.ExitCode() {
			case 130:
				return "", ErrAborted
			case 1:
				return "", ErrNoMatch
			}
		}
		return "", err
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
