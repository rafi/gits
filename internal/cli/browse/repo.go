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
	"github.com/rafi/gits/internal/cli/types"
	"github.com/rafi/gits/internal/fzf"
	"github.com/rafi/gits/internal/project"
)

const ReadMeFilename = "README.md"

// ExecRepoOverview displays a repository with README preview.
// Args:
//   - project name
//   - repo name
func ExecRepoOverview(args []string, deps types.RuntimeDeps) error {
	// Validate and load project.
	if len(args) < 1 {
		return fmt.Errorf("missing project name")
	}
	project, err := project.GetProject(args[0], deps)
	if err != nil {
		return fmt.Errorf("unable to load project %q: %w", args[0], err)
	}

	// Validate and load repo.
	if len(args) < 2 {
		return fmt.Errorf("missing repo name")
	}
	repoName := args[1]
	repo, found := project.GetRepo(repoName, "")
	if repo.Name == "" || !found {
		return fmt.Errorf("repo %q not found", repoName)
	}

	// Abort if repository is not cloned or has errors.
	if repo.State != domain.RepoStateOK {
		return cli.AbortOnRepoState(repo, deps.Theme)
	}

	// Attempt to read README file.
	readmePath := filepath.Join(repo.AbsPath, ReadMeFilename)
	readmeBytes, err := os.ReadFile(readmePath)
	if os.IsNotExist(err) {
		return fmt.Errorf("Repository does not have a %q file", ReadMeFilename)
	}
	if err != nil {
		return err
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
		return err
	}

	headerStyle := deps.Theme.PreviewHeader.Copy().PaddingLeft(2)
	if width > 0 {
		headerStyle = headerStyle.Align(lipgloss.Center).Width(width - 2)
	}

	nicePath := cli.Path(readmePath, deps.HomeDir)
	fmt.Println(headerStyle.Render(nicePath))

	// Render README as markdown.
	out, err := mkd.Render(string(readmeBytes))
	if err != nil {
		return err
	}
	fmt.Println(out)
	return nil
}
