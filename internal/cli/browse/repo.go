package browse

import (
	"fmt"
	"os"
	"path/filepath"

	"charm.land/glamour/v2"
	"charm.land/lipgloss/v2"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cli"
	"github.com/rafi/gits/internal/types"
)

const ReadMeFilename = "README.md"

// ExecRepoOverview displays a repository with README preview.
// Args:
//   - project name
//   - repo name
func ExecRepoOverview(args []string, deps types.RuntimeCLI) error {
	repo, err := resolveProjectRepo(args, deps)
	if err != nil {
		return err
	}

	// Abort if repository is not cloned or has errors.
	if repo.State != domain.RepoStateOK {
		return cli.AbortOnRepoState(repo, deps.Theme.Error)
	}

	// Attempt to read README file.
	readmePath := filepath.Join(repo.AbsPath, ReadMeFilename)
	readme, err := renderReadme(readmePath, deps)
	lipgloss.Println(readme)
	return err
}

// renderReadme renders a file as markdown.
func renderReadme(readmePath string, deps types.RuntimeCLI) (string, error) {
	readmeBytes, err := os.ReadFile(readmePath)
	if os.IsNotExist(err) {
		return "", fmt.Errorf("repository does not have a %q file", ReadMeFilename)
	}
	if err != nil {
		return "", err
	}

	width := previewWidth()

	// Initialize renderer, respect OS appearance (light/dark background).
	background := "light"
	if lipgloss.HasDarkBackground(os.Stdin, os.Stdout) {
		background = "dark"
	}
	mkd, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(background),
	)
	if err != nil {
		return "", err
	}

	headerStyle := deps.Theme.PreviewHeader.PaddingLeft(2)
	if width > 0 {
		headerStyle = previewHeader(headerStyle, width)
	}

	nicePath := cli.Path(readmePath, deps.HomeDir)
	lipgloss.Println(headerStyle.Render(nicePath))

	return mkd.Render(string(readmeBytes))
}
