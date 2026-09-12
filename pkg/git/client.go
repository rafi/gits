package git

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

// ErrNoUpstream is returned when a branch has no upstream tracking branch.
var ErrNoUpstream = errors.New("no upstream tracking branch found")

const (
	// networkTimeout is used for operations that involve network calls.
	networkTimeout = 5 * time.Minute
	// localTimeout is used for local git operations.
	localTimeout = 30 * time.Second
)

// GitClient is the set of git operations the application depends on. It is
// satisfied by the concrete *Git client and lets callers (notably the runtime
// and the bulk-command walker) be tested with a fake implementation.
type GitClient interface {
	Clone(ctx context.Context, remote string, path string) (string, error)
	IsRepo(ctx context.Context, path string) bool
	Remote(ctx context.Context, path string) (string, error)
	Fetch(ctx context.Context, path string) (string, error)
	Pull(ctx context.Context, path string) (string, error)
	Log(ctx context.Context, path, ref string) (string, error)
	CommitDates(ctx context.Context, path, branch string, days int) ([]string, error)
	Refs(ctx context.Context, path string) ([]string, error)
	Branches(ctx context.Context, path string) ([]string, error)
	Remotes(ctx context.Context, path string) ([]string, error)
	HasRemoteBranch(ctx context.Context, path, remote, branch string) bool
	Checkout(ctx context.Context, path, branch string) error
	CurrentBranch(ctx context.Context, path string) (string, error)
	UpstreamBranch(ctx context.Context, path string) (string, error)
	Modified(ctx context.Context, path string) (int, error)
	Untracked(ctx context.Context, path string) (int, error)
	CurrentPosition(ctx context.Context, path string) (string, error)
	Describe(ctx context.Context, path string) (string, error)
	Diff(ctx context.Context, path, branch, target string) (int, int, error)
}

type Git struct {
	bin string
}

// NewGit returns a new Git client.
func NewGit() (g Git, err error) {
	// Find executable path.
	g.bin, err = exec.LookPath("git")
	if err != nil {
		log.Warnf("unable to find git executable: %s", err)
	}
	return g, err
}

// Clone clones repository to filesystem.
func (g *Git) Clone(ctx context.Context, remote string, path string) (string, error) {
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		return "", fmt.Errorf("directory already exists")
	}

	basePath := filepath.Dir(path)
	if _, err := os.Stat(basePath); os.IsNotExist(err) {
		if err := os.MkdirAll(basePath, os.ModePerm); err != nil {
			return "", err
		}
		log.Debugf("Created directory %s", basePath)
	}

	ctx, cancel := context.WithTimeout(ctx, networkTimeout)
	defer cancel()

	output, err := g.Exec(ctx, basePath, []string{"clone", "--end-of-options", remote, path})
	if err != nil {
		return "", fmt.Errorf("unable to clone: %w", err)
	}
	return cleanOutput(output), nil
}

// IsRepo checks if the directory is a git repository.
func (g *Git) IsRepo(ctx context.Context, path string) bool {
	ctx, cancel := context.WithTimeout(ctx, localTimeout)
	defer cancel()

	args := []string{"rev-parse", "--is-inside-work-tree"}
	_, err := g.Exec(ctx, path, args)
	return err == nil
}

// Remote expand the URL of the current remote, taking into account any
// "url.<base>.insteadOf" config setting.
func (g *Git) Remote(ctx context.Context, path string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, localTimeout)
	defer cancel()

	args := []string{"ls-remote", "--get-url"}
	output, err := g.Exec(ctx, path, args)
	if err != nil {
		return "", fmt.Errorf("unable to get remote URL: %w", err)
	}
	return cleanOutput(output), nil
}

// Fetch fetches all remotes, tags and prunes deleted branches.
func (g *Git) Fetch(ctx context.Context, path string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, networkTimeout)
	defer cancel()

	args := []string{"fetch", "--all", "--tags", "--prune", "--force"}
	output, err := g.Exec(ctx, path, args)
	if err != nil {
		return "", fmt.Errorf("error during fetch: %w", err)
	}
	return cleanOutput(output), nil
}

// Pull fetches from remote and merges the current branch.
func (g *Git) Pull(ctx context.Context, path string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, networkTimeout)
	defer cancel()

	args := []string{"pull", "--ff-only", "--stat", "--no-verbose"}
	output, err := g.Exec(ctx, path, args)
	if err != nil {
		return "", fmt.Errorf("error during pull: %w", err)
	}
	return cleanOutput(output), nil
}

func (g *Git) Log(ctx context.Context, path, ref string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, localTimeout)
	defer cancel()

	args := []string{
		"log",
		"-15",
		"--graph",
		"--color=always",
		"--decorate",
		"--pretty=%C(240)%h%C(reset) -%C(auto)%d%Creset %s %C(242)(%an %ar)",
	}
	if len(ref) > 0 {
		args = append(args, "--end-of-options", ref)
	}
	output, err := g.Exec(ctx, path, args)
	if err != nil {
		return "", fmt.Errorf("error during log: %w", err)
	}
	return cleanOutput(output), nil
}

func (g *Git) CommitDates(ctx context.Context, path, branch string, days int) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, localTimeout)
	defer cancel()

	args := []string{
		"log",
		"--format=format:%ad",
		"--date=short",
		fmt.Sprintf("--since=%d days ago", days),
		"--end-of-options",
		branch,
	}
	output, err := g.Exec(ctx, path, args)
	if err != nil {
		return nil, fmt.Errorf("error during commit dates: %w", err)
	}
	return splitLines(cleanOutput(output)), nil
}

func (g *Git) Refs(ctx context.Context, path string) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, localTimeout)
	defer cancel()

	args := []string{
		"for-each-ref",
		"--format=%(refname)",
		"refs/heads",
		"refs/tags",
		"--sort=-committerdate",
	}
	output, err := g.Exec(ctx, path, args)
	if err != nil {
		return nil, fmt.Errorf("error during refs: %w", err)
	}
	return splitLines(cleanOutput(output)), nil
}

// Exec executes git command-line with provided arguments. On failure the
// combined stdout/stderr of git is folded into the returned error so callers
// surface the actual git message (e.g. "fatal: could not read from remote
// repository") instead of a bare "exit status 1".
func (g *Git) Exec(ctx context.Context, path string, args []string) ([]byte, error) {
	args = append([]string{"-C", path}, args...)

	cmd := exec.CommandContext(ctx, g.bin, args...)
	cmdOut, err := cmd.CombinedOutput()
	if err != nil {
		if msg := cleanOutput(cmdOut); msg != "" {
			return cmdOut, fmt.Errorf("%s: %w", msg, err)
		}
		return cmdOut, err
	}
	return cmdOut, nil
}

func cleanOutput(output []byte) string {
	b := strings.TrimSpace(string(output))
	return strings.TrimSuffix(b, "\n")
}
