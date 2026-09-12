package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"
)

// ErrNoUpstream is returned when a branch has no upstream tracking branch.
var ErrNoUpstream = errors.New("no upstream tracking branch found")

// ErrTargetExists is returned by Clone when the target directory already
// exists — usually an existing clone, or leftovers from an interrupted one.
var ErrTargetExists = errors.New(
	"directory already exists — remove it if a previous clone was interrupted")

const (
	// defaultNetworkTimeout bounds network operations (clone/fetch/pull)
	// unless overridden via SetNetworkTimeout (settings.gitTimeout).
	defaultNetworkTimeout = 5 * time.Minute
	// localTimeout is used for local read-only git queries.
	localTimeout = 30 * time.Second
	// terminateGrace is how long a signalled git process gets to clean up
	// (e.g. remove a partial clone directory) before being killed.
	terminateGrace = 10 * time.Second
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
	AllBranches(ctx context.Context, path string) ([]string, error)
	Remotes(ctx context.Context, path string) ([]string, error)
	RemoteBranches(ctx context.Context, path string) ([]string, error)
	FallbackRef(ctx context.Context, path, branch string) string
	Checkout(ctx context.Context, path, branch string) error
	CurrentBranch(ctx context.Context, path string) (string, error)
	UpstreamBranch(ctx context.Context, path string) (string, error)
	Snapshot(ctx context.Context, path string) (Snapshot, error)
	WorkingDiff(ctx context.Context, path string) (DiffStat, error)
	HeadInfo(ctx context.Context, path string) (Head, error)
	Describe(ctx context.Context, path string) (string, error)
	Diff(ctx context.Context, path, branch, target string) (int, int, error)
}

type Git struct {
	bin        string
	netTimeout time.Duration
}

// NewGit returns a new Git client.
func NewGit() (g Git, err error) {
	g.netTimeout = defaultNetworkTimeout
	// Find executable path.
	g.bin, err = exec.LookPath("git")
	if err != nil {
		log.Warnf("unable to find git executable: %s", err)
	}
	return g, err
}

// SetNetworkTimeout overrides the timeout applied to network operations
// (clone/fetch/pull); non-positive values keep the default.
func (g *Git) SetNetworkTimeout(d time.Duration) {
	if d > 0 {
		g.netTimeout = d
	}
}

// networkTimeout returns the effective network-operation timeout, guarding
// zero-value Git instances built without NewGit.
func (g *Git) networkTimeout() time.Duration {
	if g.netTimeout <= 0 {
		return defaultNetworkTimeout
	}
	return g.netTimeout
}

// Clone clones repository to filesystem.
func (g *Git) Clone(ctx context.Context, remote string, path string) (string, error) {
	if _, err := os.Stat(path); err == nil {
		return "", ErrTargetExists
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}

	basePath := filepath.Dir(path)
	if _, err := os.Stat(basePath); os.IsNotExist(err) {
		if err := os.MkdirAll(basePath, os.ModePerm); err != nil {
			return "", err
		}
		log.Debugf("Created directory %s", basePath)
	}

	ctx, cancel := context.WithTimeout(ctx, g.networkTimeout())
	defer cancel()

	output, err := g.ExecCombined(ctx, basePath, []string{"clone", "--end-of-options", remote, path})
	if err != nil {
		return "", fmt.Errorf("unable to clone: %w", err)
	}
	return cleanOutput(output), nil
}

// IsRepo checks if the directory is a git repository root. Detection is by
// .git presence — a directory, or a gitfile for worktrees and submodules —
// so callers probing many candidates (walks, project population) pay a stat
// instead of a subprocess per directory.
func (g *Git) IsRepo(_ context.Context, path string) bool {
	_, err := os.Stat(filepath.Join(path, ".git"))
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
	ctx, cancel := context.WithTimeout(ctx, g.networkTimeout())
	defer cancel()

	args := []string{"fetch", "--all", "--tags", "--prune", "--force"}
	output, err := g.ExecCombined(ctx, path, args)
	if err != nil {
		return "", fmt.Errorf("error during fetch: %w", err)
	}
	return cleanOutput(output), nil
}

// Pull fetches from remote and merges the current branch.
func (g *Git) Pull(ctx context.Context, path string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, g.networkTimeout())
	defer cancel()

	args := []string{"pull", "--ff-only", "--stat", "--no-verbose"}
	output, err := g.ExecCombined(ctx, path, args)
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

// command builds the git invocation for path; every execution path goes
// through here. On context cancellation git is terminated gracefully
// (SIGTERM with a kill grace period) so its own cleanup handlers run —
// e.g. removing a partially cloned directory — instead of the default
// SIGKILL which leaves junk behind.
func (g *Git) command(ctx context.Context, path string, args []string) *exec.Cmd {
	args = append([]string{"-C", path}, args...)
	cmd := exec.CommandContext(ctx, g.bin, args...)
	cmd.Cancel = func() error {
		if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
			return cmd.Process.Kill()
		}
		return nil
	}
	cmd.WaitDelay = terminateGrace
	return cmd
}

// Exec executes git command-line with provided arguments and returns stdout
// only, so stderr noise (warnings, traces) can never corrupt parse paths
// like the status porcelain. On failure git's stderr (or stdout when stderr
// is empty) is folded into the returned error so callers surface the actual
// git message (e.g. "fatal: could not read from remote repository") instead
// of a bare "exit status 1"; on success stderr is debug-logged.
func (g *Git) Exec(ctx context.Context, path string, args []string) ([]byte, error) {
	var stdout, stderr bytes.Buffer
	cmd := g.command(ctx, path, args)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := cleanOutput(stderr.Bytes())
		if msg == "" {
			msg = cleanOutput(stdout.Bytes())
		}
		if msg != "" {
			return stdout.Bytes(), fmt.Errorf("%s: %w", msg, err)
		}
		return stdout.Bytes(), err
	}
	if msg := cleanOutput(stderr.Bytes()); msg != "" {
		log.Debugf("git -C %s: stderr: %s", path, msg)
	}
	return stdout.Bytes(), nil
}

// ExecCombined executes git and returns stdout and stderr interleaved, for
// user-facing output of commands like clone/fetch/pull that write progress
// and summaries to stderr. Never use it for output that gets parsed.
func (g *Git) ExecCombined(ctx context.Context, path string, args []string) ([]byte, error) {
	cmdOut, err := g.command(ctx, path, args).CombinedOutput()
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
