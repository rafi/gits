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

// ReadMeFilename is the file the repository overview previews.
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
		return cli.AbortOnRepoState(deps.Err, repo, deps.Theme.Error)
	}

	// Attempt to read README file.
	readmePath := filepath.Join(repo.AbsPath, ReadMeFilename)
	// Nothing rendered means nothing to show: a repository with no README used
	// to put a bare newline on Result Output, which is chrome in the one place
	// that promises to carry only what the command was asked for.
	readme, err := renderReadme(readmePath, deps)
	if err != nil {
		return err
	}
	lipgloss.Fprintln(deps.Out, readme)
	return nil
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
	// The probe writes a query to Result Output's destination and reads the
	// terminal's reply on the input stream, so it has to ask the destination
	// rather than the process stream. Both sides must be files: a captured
	// destination cannot be asked and keeps the default.
	background := "light"
	if out, ok := deps.Out.(*os.File); ok && lipgloss.HasDarkBackground(os.Stdin, out) {
		background = "dark"
	}
	mkd, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(background),
	)
	if err != nil {
		return "", err
	}

	headerStyle := deps.Theme.PreviewHeader.PaddingLeft(previewHeaderPad)
	if width > 0 {
		headerStyle = previewHeader(headerStyle, width)
	}

	nicePath := cli.Path(readmePath, deps.HomeDir)
	lipgloss.Fprintln(deps.Out, headerStyle.Render(nicePath))

	return mkd.Render(string(readmeBytes))
}
