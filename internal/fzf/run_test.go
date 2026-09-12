package fzf

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
