// Package exec runs an arbitrary command in one repository, reporting what
// the child said rather than a line to print.
//
// Three scope decisions, taken from the ticket's open questions:
//
//   - Shell. The command is run as argv, never through a shell, so nothing a
//     repository name or a path contains can be re-interpreted as syntax. A
//     user who wants pipes, redirection or globbing writes the shell
//     themselves: `gits exec acme -- sh -c 'git log -1 | cat'`.
//   - Environment. The child inherits the parent environment plus GITS_PROJECT,
//     GITS_REPO and GITS_REPO_PATH, so a `sh -c` body can name the repository
//     it is running in without re-deriving it.
//   - Timeout. None of its own. settings.gitTimeout bounds gits' own network
//     git operations and says nothing about an arbitrary user command, which
//     may legitimately run for as long as it likes. The root context still
//     cancels every in-flight child on Ctrl-C.
package exec

import (
	"context"
	"errors"
	"fmt"
	"os"
	osexec "os/exec"
	"strings"

	"github.com/rafi/gits/internal/runtime/command"
)

// ErrNoCommand is returned when no command follows the `--` separator.
var ErrNoCommand = errors.New("no command to run: gits exec [project] [repo] -- <command> [args...]")

// Report is what running the command in one repository produced.
type Report struct {
	// Output is the child's combined output, trimmed and unstyled.
	Output string
}

// Repo runs command in one repository. A non-zero exit is the repository's
// error — reported on its line and in the error epilogue — with whatever the
// child said kept alongside the exit status, since a bare "exit status 1"
// names nothing the user can act on.
func Repo(ctx context.Context, command []string, repo command.Repo) (Report, error) {
	// gosec G204: the argv is the user's own command, typed on their own
	// command line. Running it is the entire feature, and it is run as argv
	// rather than through a shell precisely so that nothing else — a path, a
	// repository name — can add to it.
	cmd := osexec.CommandContext(ctx, command[0], command[1:]...) //nolint:gosec // the user's own argv
	cmd.Dir = repo.AbsPath
	cmd.Env = append(os.Environ(),
		"GITS_PROJECT="+repo.Project.Name,
		"GITS_REPO="+repo.GetName(),
		"GITS_REPO_PATH="+repo.AbsPath,
	)

	out, err := cmd.CombinedOutput()
	body := strings.TrimRight(string(out), "\n")
	if err != nil {
		if body == "" {
			return Report{}, err
		}
		return Report{}, fmt.Errorf("%w: %s", err, body)
	}
	return Report{Output: body}, nil
}
