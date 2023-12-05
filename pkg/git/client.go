package git

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/transport/client"
	"github.com/go-git/go-git/v5/plumbing/transport/file"
	log "github.com/sirupsen/logrus"
)

type Git struct {
	bin string
}

// NewGit returns a new Git client.
func NewGit() (g Git, err error) {
	// Find executable path.
	client.InstallProtocol("file", file.DefaultClient)

	g.bin, err = exec.LookPath("git")
	if err != nil {
		log.Warnf("unable to find git executable: %s", err)
	}
	return g, nil
}

// Clone clones repository to filesystem.
func (g Git) Clone(remote string, path string) (string, error) {
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		return "", fmt.Errorf("directory already exists")
	}

	basePath := filepath.Dir(path)
	if _, err := os.Stat(basePath); os.IsNotExist(err) {
		os.MkdirAll(basePath, os.ModePerm)
		log.Debugf("Created directory %s", basePath)
	}

	args := []string{"clone", remote, path}
	output, err := g.Exec(basePath, args)
	if err != nil {
		return "", fmt.Errorf("unable to clone: %w", err)
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
func (g Git) IsRepo(path string) bool {
	_, err := git.PlainOpen(path)
	return err == nil
}

// Remote expand the URL of the current remote, taking into account any
// "url.<base>.insteadOf" config setting.
func (g Git) Remote(path string) (string, error) {
	args := []string{"ls-remote", "--get-url"}
	output, err := g.Exec(path, args)
	if err != nil {
		return "", fmt.Errorf("unable to get remote URL: %w", err)
	}
	return cleanOutput(output), nil
}

// Fetch fetches all remotes, tags and prunes deleted branches.
func (g Git) Fetch(path string) (string, error) {
	args := []string{"fetch", "--all", "--tags", "--prune", "--force"}
	output, err := g.Exec(path, args)
	if err != nil {
		return "", fmt.Errorf("error during fetch: %w", err)
	}
	return cleanOutput(output), nil
}

// Exec executes git command-line with provided arguments.
func (g Git) Exec(path string, args []string) ([]byte, error) {
	var (
		cmdOut []byte
		err    error
	)
	args = append([]string{"-C", path}, args...)

	cmd := exec.Command(g.bin, args...)
	if cmdOut, err = cmd.CombinedOutput(); err != nil {
		return cmdOut, err
	}
	return cmdOut, nil
}

func cleanOutput(output []byte) string {
	return strings.TrimSuffix(string(output), "\n")
}
