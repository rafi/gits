package fzf

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/rafi/gits/domain"
)

// stubFzf installs a fake fzf binary at the front of PATH.
func stubFzf(t *testing.T, script string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "fzf")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script+"\n"), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

//nolint:paralleltest // stubFzf calls t.Setenv, which is incompatible with t.Parallel.
func TestRunExitCodeMapping(t *testing.T) {
	ctx := context.Background()

	t.Run("exit 130 is ErrAborted", func(t *testing.T) {
		stubFzf(t, "exit 130")
		_, err := New(io.Discard).Run(ctx, bytes.Buffer{})
		if !errors.Is(err, ErrAborted) {
			t.Errorf("Run error = %v, want ErrAborted", err)
		}
	})

	t.Run("exit 1 is ErrNoMatch", func(t *testing.T) {
		stubFzf(t, "exit 1")
		_, err := New(io.Discard).Run(ctx, bytes.Buffer{})
		if !errors.Is(err, ErrNoMatch) {
			t.Errorf("Run error = %v, want ErrNoMatch", err)
		}
	})

	t.Run("exit 2 stays a plain error", func(t *testing.T) {
		stubFzf(t, "exit 2")
		_, err := New(io.Discard).Run(ctx, bytes.Buffer{})
		if err == nil || errors.Is(err, ErrAborted) || errors.Is(err, ErrNoMatch) {
			t.Errorf("Run error = %v, want plain exec error", err)
		}
	})

	t.Run("selection is trimmed and returned", func(t *testing.T) {
		stubFzf(t, "echo ' picked '")
		got, err := New(io.Discard).Run(ctx, bytes.Buffer{})
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		if got != "picked" {
			t.Errorf("Run = %q, want %q", got, "picked")
		}
	})
}

// TestRunDrawsOnDiagnostics proves the finder's own interface goes to the
// destination it was constructed with rather than to the process stream, so
// nothing fzf draws can land on Result Output.
//
//nolint:paralleltest // stubFzf calls t.Setenv, which is incompatible with t.Parallel.
func TestRunDrawsOnDiagnostics(t *testing.T) {
	stubFzf(t, "echo drawing >&2; echo picked")
	diagnostics := bytes.Buffer{}

	got, err := New(&diagnostics).Run(context.Background(), bytes.Buffer{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got != "picked" {
		t.Errorf("Run = %q, want %q", got, "picked")
	}
	if diagnostics.String() != "drawing\n" {
		t.Errorf("diagnostics = %q, want %q", diagnostics.String(), "drawing\n")
	}
}

// TestRunCallerArgsWin proves caller args land after the defaults, so a
// caller can override a default (fzf is last-flag-wins), and that Run does
// not mutate the caller's Args slice.
//
//nolint:paralleltest // stubFzf calls t.Setenv, which is incompatible with t.Parallel.
func TestRunCallerArgsWin(t *testing.T) {
	stubFzf(t, `printf '%s\n' "$@"`)
	f := New(io.Discard, "--multi")
	before := fmt.Sprintf("%v", f.Args)

	got, err := f.Run(context.Background(), bytes.Buffer{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	args := strings.Split(got, "\n")
	multiAt, noMultiAt := -1, -1
	for i, a := range args {
		switch a {
		case "--multi":
			multiAt = i
		case "--no-multi":
			noMultiAt = i
		}
	}
	if multiAt == -1 || noMultiAt == -1 {
		t.Fatalf("args missing markers: %v", args)
	}
	if multiAt < noMultiAt {
		t.Errorf("caller --multi at %d before default --no-multi at %d; caller must win", multiAt, noMultiAt)
	}
	if after := fmt.Sprintf("%v", f.Args); after != before {
		t.Errorf("Run mutated caller Args: %v -> %v", before, after)
	}
}

// stubNamedBinary installs an executable named bin at the front of PATH,
// echoing its own arguments one per line. It lets a test assert which binary
// was invoked and with which options.
func stubNamedBinary(t *testing.T, bin string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, bin)
	if err := os.WriteFile(path, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\"\n"), 0o755); err != nil {
		t.Fatalf("write stub %q: %v", bin, err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// TestRunFinderOverrides proves settings.finder reaches the invocation: a
// non-empty Binary replaces the fzf executable, a non-empty Args replaces the
// built-in default options, and Extra is always appended.
//
//nolint:paralleltest // stubNamedBinary calls t.Setenv, incompatible with t.Parallel.
func TestRunFinderOverrides(t *testing.T) {
	ctx := context.Background()

	t.Run("binary overrides fzf", func(t *testing.T) {
		stubNamedBinary(t, "myfinder")
		out, err := New(io.Discard).
			WithFinder(domain.Finder{Binary: "myfinder"}).
			Run(ctx, bytes.Buffer{})
		if err != nil {
			t.Fatalf("Run with custom binary: %v", err)
		}
		// The stub echoes only its args, so a clean run proves it was the one
		// invoked; a missing binary would have failed LookPath instead.
		_ = out
	})

	t.Run("args replace defaults and extra appends", func(t *testing.T) {
		t.Setenv("FZF_DEFAULT_OPTS", "set") // suppress the default layout opts
		stubFzf(t, `printf '%s\n' "$@"`)

		out, err := New(io.Discard, "--nth=1").
			WithFinder(domain.Finder{
				Args:  []string{"--custom-default"},
				Extra: []string{"--extra-flag"},
			}).
			Run(ctx, bytes.Buffer{})
		if err != nil {
			t.Fatalf("Run with finder args: %v", err)
		}
		args := strings.Split(strings.TrimSpace(out), "\n")

		if slices.Contains(args, "--no-multi") {
			t.Errorf("args = %v, want the built-in defaultOpts replaced by finder.Args", args)
		}
		for _, want := range []string{"--custom-default", "--extra-flag", "--nth=1"} {
			if !slices.Contains(args, want) {
				t.Errorf("args = %v, want it to contain %q", args, want)
			}
		}
		// Extra precedes the caller's own args, which stay last so they win.
		if slices.Index(args, "--extra-flag") > slices.Index(args, "--nth=1") {
			t.Errorf("args = %v, want caller args after finder.Extra", args)
		}
	})
}
