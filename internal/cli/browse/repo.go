package browse

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	log "github.com/sirupsen/logrus"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cli"
	"github.com/rafi/gits/internal/loader"
	"github.com/rafi/gits/internal/types"
	"github.com/rafi/gits/pkg/fzf"
)

const ReadMeFilename = "README.md"

// ExecRepoOverview displays a repository with README preview.
// Args:
//   - project name
//   - repo name
func ExecRepoOverview(args []string, deps types.RuntimeCLI) error {
	// Validate and load project.
	if len(args) < 1 {
		return fmt.Errorf("missing project name")
	}
	project, err := loader.GetProject(args[0], deps.Runtime)
	if err != nil {
		return fmt.Errorf("unable to load project %q: %w", args[0], err)
	}

	// Validate and load repo.
	if len(args) < 2 {
		return fmt.Errorf("missing repo name")
	}
	repoName := args[1]
	repo, found := project.GetRepo(repoName, "")
	if !found {
		return fmt.Errorf("repo %s/%s not found", args[0], repoName)
	}

	// Abort if repository is not cloned or has errors.
	if repo.State != domain.RepoStateOK {
		return cli.AbortOnRepoState(repo, deps.Theme.Error)
	}

	// Attempt to read README file.
	readmePath := filepath.Join(repo.AbsPath, ReadMeFilename)
	readme, err := renderReadme(readmePath, deps)
	fmt.Println(readme)
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

	// Fzf sets environment variables to detect width/height, see man fzf.
	width, _, err := fzf.GetPreviewSize()
	if err != nil {
		log.Warnf("unable to parse FZF_PREVIEW_COLUMNS: %s", err)
	}

	// Initialize renderer, respect OS appearance (light/dark background).
	background := "light"
	if lipgloss.HasDarkBackground() {
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
		headerStyle = headerStyle.Align(lipgloss.Center).Width(width - 2)
	}

	nicePath := cli.Path(readmePath, deps.HomeDir)
	fmt.Println(headerStyle.Render(nicePath))

	return mkd.Render(string(readmeBytes))
}
