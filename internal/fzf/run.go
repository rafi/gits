// Package fzf drives the fzf binary for interactive selection of Projects,
// Repositories and branches.
package fzf

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/rafi/gits/domain"
)

const fzfBin = "fzf"

var (
	// ErrAborted is returned when the user cancels the finder (Esc/Ctrl-C).
	ErrAborted = errors.New("selection aborted")
	// ErrNoMatch is returned when fzf exits without a match.
	ErrNoMatch = errors.New("no match")
)

const (
	// exitInterrupted is what fzf exits with when the user aborts the
	// selection — the shell's 128 + SIGINT.
	exitInterrupted = 130
	// previewSizeFields is how many size variables must be set before a
	// preview size is known: width and height.
	previewSizeFields = 2
)

// FZF is one configured fzf invocation.
type FZF struct {
	Args []string

	// finder overrides the binary and default options from config
	// (settings.finder). Its zero value keeps the built-in fzf binary and
	// defaultOpts.
	finder domain.Finder

	// diagnostics is where fzf draws its own interface. That is Diagnostic
	// Output: the finder is chrome around a selection, and the selection
	// itself is returned rather than printed, so nothing here belongs on
	// Result Output.
	diagnostics io.Writer
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

// New returns a finder that draws its interface on diagnostics. Passing the
// process's own diagnostic stream hands fzf the terminal directly, which is
// what it needs to render; any other writer hides the interface behind a
// pipe while fzf still waits for a selection.
func New(diagnostics io.Writer, args ...string) *FZF {
	return &FZF{Args: args, diagnostics: diagnostics}
}

// WithFinder applies the config's settings.finder overrides: a non-empty
// Binary replaces the fzf executable, a non-empty Args replaces defaultOpts,
// and Extra is always appended. Returns the same finder for chaining.
func (f *FZF) WithFinder(finder domain.Finder) *FZF {
	f.finder = finder
	return f
}

// WithPreview adds a preview command to the finder, with opts sizing the
// preview window; an empty opts uses the default layout.
func (f *FZF) WithPreview(cmd, opts string) {
	f.Args = append(f.Args, "--preview", cmd)
	if opts == "" {
		opts = defaultPreviewOpts
	}
	f.Args = append(f.Args, "--preview-window", opts)
}

// WithPrompt sets the finder's prompt label.
func (f *FZF) WithPrompt(label string) {
	f.Args = append(f.Args, "--prompt", label)
}

// Run executes fzf with given args and stdin. Canceling by the user maps to
// ErrAborted, and an empty match to ErrNoMatch, so callers can exit quietly.
func (f *FZF) Run(ctx context.Context, stdin bytes.Buffer) (string, error) {
	bin := fzfBin
	if f.finder.Binary != "" {
		bin = f.finder.Binary
	}
	if _, err := exec.LookPath(bin); err != nil {
		return "", fmt.Errorf("%s not found in PATH", bin)
	}

	// The default option set, which the config may replace wholesale via
	// settings.finder.args. Defaults come first so any conflicting caller
	// option wins (fzf is last-flag-wins); copying also avoids aliasing.
	base := defaultOpts
	if len(f.finder.Args) > 0 {
		base = f.finder.Args
	}
	args := append([]string{}, base...)
	if os.Getenv("FZF_DEFAULT_OPTS") == "" {
		args = append(args, defaultLayoutOpts...)
	}
	// settings.finder.extra is always appended, on top of args or a replaced base.
	args = append(args, f.finder.Extra...)
	args = append(args, f.Args...)

	// Run shell command with stdin
	var cmdOut bytes.Buffer
	fzf := exec.CommandContext(ctx, bin, args...)
	fzf.Stdin = &stdin
	fzf.Stdout = &cmdOut
	fzf.Stderr = f.diagnostics
	if err := fzf.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			switch exitErr.ExitCode() {
			case exitInterrupted:
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
	if len(sizes) < previewSizeFields {
		return 0, 0, nil
	}
	return sizes[0], sizes[1], nil
}
