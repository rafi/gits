package checkout

import (
	"fmt"

	"charm.land/lipgloss/v2"

	"github.com/rafi/gits/internal/app"
)

// The three outcomes a prompted repository can have, each one line on Result
// Output, opened by the repository's title.

// renderUnchanged reports a pick that changes nothing: the branch stays as it
// was, and its name is the whole line.
func renderUnchanged(repoTitle, current string, deps app.RuntimeCLI) {
	lipgloss.Fprintf(deps.Out, "%s %s\n", repoTitle, current)
}

// renderSwitched reports a completed checkout.
func renderSwitched(repoTitle, want string, deps app.RuntimeCLI) {
	lipgloss.Fprintf(deps.Out, "%s %s\n", repoTitle, deps.Theme.GitOutput.Render(
		fmt.Sprintf("Switched to branch %q", want),
	))
}

// renderFailure reports a checkout git refused. Titled and terminated like the
// two lines above: this used to trail the prompt's own final line, which no
// longer exists.
func renderFailure(repoTitle string, err error, deps app.RuntimeCLI) {
	lipgloss.Fprintf(deps.Out, "%s %s\n", repoTitle, deps.Theme.Error.Render(err.Error()))
}
