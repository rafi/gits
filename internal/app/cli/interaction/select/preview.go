package pick

import (
	"fmt"
	"os"
	"strings"
	"sync"
)

// selfPath resolves the running binary's own path once per process, for the
// fzf preview commands that re-invoke gits. Preview commands run through a
// shell, so a binary that was renamed, installed outside PATH or started by
// `go run` would otherwise render "command not found" in every preview pane.
// [os.Executable] failing leaves the bare name as the only thing left to try.
var selfPath = sync.OnceValue(func() string { return resolveSelf(os.Executable) })

// resolveSelf picks the name a preview command invokes gits by: the running
// binary's own path, or the bare name when the lookup fails or comes back
// empty, which is all that is left to try.
func resolveSelf(executable func() (string, error)) string {
	exe, err := executable()
	if err != nil || exe == "" {
		return "gits"
	}
	return exe
}

// shellQuote renders s as a single sh word. Everything is wrapped in single
// quotes, with embedded single quotes spliced in as '\” — the one character
// single quoting cannot carry.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// previewCommand builds an fzf preview command that re-invokes gits: the
// resolved path to this binary, the shared flags, the sub-command, and its
// arguments, each shell-quoted. themeName is the caller's already-resolved
// theme (deps.Scheme.Theme.Name), passed through with --theme so the preview
// child renders in the same theme as the parent rather than re-resolving one
// of its own: without it, the child would fall back to the config's theme
// (ignoring an active --theme/GITS_THEME override) and, for a configured
// {dark, light} pair, could pick the wrong half — its stdout is a pipe to
// fzf, which a background probe always answers as dark. Anything fzf must
// expand itself — {1}, {2}, {} — is appended by the caller, outside the
// quoting.
func previewCommand(configPath, themeName, subCmd string, args ...string) string {
	// The binary, the shared flags (color, config, theme) and the sub-command
	// precede args.
	const fixedParts = 5

	// themeName is not quoted: it is always a resolved theme's registry name
	// (deps.Scheme.Theme.Name), never arbitrary input, so it carries no shell
	// metacharacters — same as the -C=always flag beside it.
	parts := make([]string, 0, len(args)+fixedParts)
	parts = append(parts,
		shellQuote(selfPath()), "-C=always", "--config="+shellQuote(configPath),
		"--theme="+themeName, subCmd)
	for _, arg := range args {
		parts = append(parts, shellQuote(arg))
	}
	return strings.Join(parts, " ")
}

// previewCommandf is previewCommand with a trailing fzf placeholder expression
// appended verbatim, so the placeholder stays outside the quoted arguments.
func previewCommandf(configPath, themeName, subCmd, trailing string, args ...string) string {
	return fmt.Sprintf("%s %s", previewCommand(configPath, themeName, subCmd, args...), trailing)
}
