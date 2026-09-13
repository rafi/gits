package doctor

import (
	"fmt"

	"charm.land/lipgloss/v2"

	"github.com/rafi/gits/internal/app"
	"github.com/rafi/gits/internal/app/cli/output"
	"github.com/rafi/gits/internal/service/health"
	"github.com/rafi/gits/internal/service/wire"
)

// render writes the report as Result Output. The findings are the result of
// this command — the thing to pipe into a grep or a ticket — so they go to
// Out, not Err, even though every one of them is about something being wrong.
func render(format string, report Report, deps app.RuntimeCLI) error {
	if format == output.FormatJSON {
		return wire.WriteValue(deps.Out, report)
	}

	for _, f := range report.Findings {
		lipgloss.Fprintf(deps.Out, "%s %s %s\n",
			levelStyle(f.Level, deps).Render(fmt.Sprintf("%-7s", f.Level)),
			deps.Theme.RepoTitle.Render(f.Subject),
			f.Message,
		)
	}
	return nil
}

// levelStyle maps a level to how it is shown. The mapping runs this way round:
// the level strings are the contract, and color is a display choice made here.
func levelStyle(level health.Level, deps app.RuntimeCLI) lipgloss.Style {
	switch level {
	case health.LevelError:
		return deps.Theme.Error
	case health.LevelWarning:
		return deps.Theme.Warning
	case health.LevelInfo:
		return deps.Theme.StatusDim
	}
	return deps.Theme.Normal
}
