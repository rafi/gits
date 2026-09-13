// Package doctor is the view side of `gits doctor`: it runs the health checks
// and renders their findings as a table or as JSON.
package doctor

import (
	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/app"
	"github.com/rafi/gits/internal/app/cli/output"
	"github.com/rafi/gits/internal/service/health"
)

// ExecDoctor runs every check and renders the report.
//
// It takes no project argument by design: "which of my projects is
// misconfigured" is the question, so scoping the answer to a project the user
// already suspects would defeat it.
func ExecDoctor(format string, _ []string, deps app.RuntimeCLI) error {
	// doctor runs nothing across repositories, but it renders the same two
	// formats, so it shares the one validator rather than keeping a second
	// copy of the pair that could drift from the flag's completion and help.
	if err := output.ValidateFormat(format); err != nil {
		return err
	}

	report := Report{
		ConfigPath: deps.ConfigPath,
		Findings:   health.Check(deps.Runtime),
	}
	if err := render(format, report, deps); err != nil {
		return err
	}
	if health.HasErrors(report.Findings) {
		// The message is the report itself, which the user is looking at.
		// Returning a bare non-zero rather than an epilogue keeps the last
		// line of output a finding rather than a restatement.
		return domain.ErrSilent
	}
	return nil
}

// Report is the document `-o json` emits: the findings, plus the config file
// they were made against.
type Report struct {
	ConfigPath string           `json:"configPath"`
	Findings   []health.Finding `json:"findings"`
}
