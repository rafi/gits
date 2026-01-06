package git

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v6"
	log "github.com/sirupsen/logrus"
)

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
	return g, nil
}

// Clone clones repository to filesystem.
func (g *Git) Clone(remote string, path string) (string, error) {
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

	args := []string{"clone", remote, path}
	output, err := g.Exec(basePath, args)
	if err != nil {
		return "", fmt.Errorf("unable to clone: %s %w", output, err)
	}
	return cleanOutput(output), nil
}

// Open returns a git repository client.
func (g *Git) Open(path string) (repo Repository, err error) {
	repo.client, err = git.PlainOpen(path)
	if err != nil {
		return repo, fmt.Errorf("failed to open git repository: %w", err)
	}
	return repo, nil
}

// IsRepo checks if the directory is a git repository.
func (g *Git) IsRepo(path string) bool {
	_, err := git.PlainOpen(path)
	return err == nil
}

// Remote expand the URL of the current remote, taking into account any
// "url.<base>.insteadOf" config setting.
func (g *Git) Remote(path string) (string, error) {
	args := []string{"ls-remote", "--get-url"}
	output, err := g.Exec(path, args)
	if err != nil {
		return "", fmt.Errorf("unable to get remote URL: %w", err)
	}
	return cleanOutput(output), nil
}

// Fetch fetches all remotes, tags and prunes deleted branches.
func (g *Git) Fetch(path string) (string, error) {
	args := []string{"fetch", "--all", "--tags", "--prune", "--force"}
	output, err := g.Exec(path, args)
	if err != nil {
		return "", fmt.Errorf("error during fetch: %w", err)
	}
	return cleanOutput(output), nil
}

// Pull fetches from remote and merges the current branch.
func (g *Git) Pull(path string) (string, error) {
	args := []string{"pull", "--ff-only", "--stat", "--no-verbose"}
	output, err := g.Exec(path, args)
	if err != nil {
		return "", fmt.Errorf("error during pull: %w", err)
	}
	return cleanOutput(output), nil
}

func (g *Git) Log(path, ref string) (string, error) {
	args := []string{
		"log",
		"-15",
		"--graph",
		"--color=always",
		"--decorate",
		"--pretty=%C(240)%h%C(reset) -%C(auto)%d%Creset %s %C(242)(%an %ar)",
	}
	if len(ref) > 0 {
		args = append(args, ref)
	}
	output, err := g.Exec(path, args)
	if err != nil {
		return "", fmt.Errorf("error during log: %w", err)
	}
	return cleanOutput(output), nil
}

func (g *Git) CommitDates(path, branch string, days int) ([]string, error) {
	args := []string{
		"log",
		"--format=format:%ad",
		"--date=short",
		fmt.Sprintf("--since=%d days ago", days),
		branch,
	}
	output, err := g.Exec(path, args)
	if err != nil {
		return nil, fmt.Errorf("error during commit dates: %w", err)
	}
	return strings.Split(cleanOutput(output), "\n"), nil
}

func (g *Git) Refs(path string) ([]string, error) {
	args := []string{
		"for-each-ref",
		"--format=%(refname)",
		"refs/heads",
		"refs/tags",
		"--sort=-committerdate",
	}
	output, err := g.Exec(path, args)
	if err != nil {
		return nil, fmt.Errorf("error during commit dates: %w", err)
	}
	return strings.Split(cleanOutput(output), "\n"), nil
}

// Exec executes git command-line with provided arguments.
func (g *Git) Exec(path string, args []string) ([]byte, error) {
	var (
		cmdOut []byte
		err    error
	)
	args = append([]string{"-C", path}, args...)

	cmd := exec.CommandContext(context.TODO(), g.bin, args...)
	if cmdOut, err = cmd.CombinedOutput(); err != nil {
		return cmdOut, err
	}
	return cmdOut, nil
}

func cleanOutput(output []byte) string {
	b := strings.TrimSpace(string(output))
	return strings.TrimSuffix(b, "\n")
}
