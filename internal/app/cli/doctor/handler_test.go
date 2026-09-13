package doctor

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/app/cli/clitest"
)

// Every test drives ExecDoctor — the command's real entry point — with a nil
// git client, because doctor must never reach one: any git call would be a bug
// the nil client turns into a panic.

// TestDoctorExitCode pins the rule tickets 01 and 02 established: an error
// finding exits non-zero so CI can gate on it, and warnings and info do not.
func TestDoctorExitCode(t *testing.T) {
	t.Parallel()

	t.Run("an error finding fails the run", func(t *testing.T) {
		t.Parallel()

		deps := clitest.New(t, nil)
		deps.Projects = domain.ProjectListKeyed{"acme": {
			Repos: []domain.Repository{{Name: "rel", Dir: "relative-dir"}},
		}}

		err := ExecDoctor("table", nil, deps.RuntimeCLI)
		if err == nil {
			t.Fatal("ExecDoctor = nil, want a non-zero exit for an error finding")
		}
		// The findings are the output, so the failure carries no message of
		// its own to restate them.
		if !domain.IsSilent(err) {
			t.Errorf("error = %v, want a silent one", err)
		}
	})

	t.Run("warnings alone succeed", func(t *testing.T) {
		t.Parallel()

		deps := clitest.New(t, nil)
		deps.Projects = domain.ProjectListKeyed{
			"acme": {Path: filepath.Join(t.TempDir(), "not-created-yet")},
		}

		if err := ExecDoctor("table", nil, deps.RuntimeCLI); err != nil {
			t.Errorf("ExecDoctor = %v, want nil: a warning is not a failure", err)
		}
	})
}

// TestDoctorWritesResultOutput proves the findings are Result Output. They are
// what the command was asked for — the thing to pipe into a grep or a ticket —
// even though every one of them is about something being wrong.
func TestDoctorWritesResultOutput(t *testing.T) {
	t.Parallel()

	for _, format := range []string{"table", "json"} {
		t.Run(format, func(t *testing.T) {
			t.Parallel()

			deps := clitest.New(t, nil)
			deps.Projects = domain.ProjectListKeyed{"acme": {Path: t.TempDir()}}

			if err := ExecDoctor(format, nil, deps.RuntimeCLI); err != nil {
				t.Fatalf("ExecDoctor: %v", err)
			}
			if deps.Result() == "" {
				t.Error("Result Output empty, want the findings")
			}
			if got := deps.Diagnostic(); got != "" {
				t.Errorf("Diagnostic Output = %q, want empty: findings are Result Output", got)
			}
		})
	}
}

// TestDoctorTableLine proves each rendered line carries the level, the subject
// and the message, in that order, so the table stays greppable.
func TestDoctorTableLine(t *testing.T) {
	t.Parallel()

	deps := clitest.New(t, nil)
	deps.Projects = domain.ProjectListKeyed{"acme": {
		Repos: []domain.Repository{{Name: "rel", Dir: "relative-dir"}},
	}}

	if err := ExecDoctor("table", nil, deps.RuntimeCLI); err != nil && !domain.IsSilent(err) {
		t.Fatalf("ExecDoctor: %v", err)
	}

	var line string
	for l := range strings.SplitSeq(deps.Result(), "\n") {
		if strings.Contains(l, "acme.rel") {
			line = l
		}
	}
	if line == "" {
		t.Fatalf("Result Output = %q, want a line for the repository", deps.Result())
	}
	fields := strings.Fields(line)
	if len(fields) < 3 || fields[0] != "error" || fields[1] != "acme.rel" {
		t.Errorf("line = %q, want the level then the subject then the message", line)
	}
}

// TestDoctorJSONDocument pins the wire shape: one newline-terminated line, so
// it pipes into jq without a reader knowing how many lines to expect.
func TestDoctorJSONDocument(t *testing.T) {
	t.Parallel()

	deps := clitest.New(t, nil)
	deps.ConfigPath = "/home/nobody/.gits.yaml"
	deps.Projects = domain.ProjectListKeyed{"acme": {
		Repos: []domain.Repository{{Name: "rel", Dir: "relative-dir"}},
	}}

	if err := ExecDoctor("json", nil, deps.RuntimeCLI); err != nil && !domain.IsSilent(err) {
		t.Fatalf("ExecDoctor: %v", err)
	}

	raw := deps.Result()
	if n := strings.Count(raw, "\n"); n != 1 || !strings.HasSuffix(raw, "\n") {
		t.Fatalf("Result Output = %q, want one newline-terminated line", raw)
	}

	// Decoded structurally rather than through the report types, so the wire
	// contract is checked against something other than itself.
	var doc struct {
		ConfigPath string `json:"configPath"`
		Findings   []struct {
			Level   string `json:"level"`
			Scope   string `json:"scope"`
			Subject string `json:"subject"`
			Message string `json:"message"`
		} `json:"findings"`
	}
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("unmarshal Result Output %q: %v", raw, err)
	}
	if doc.ConfigPath != deps.ConfigPath {
		t.Errorf("configPath = %q, want %q", doc.ConfigPath, deps.ConfigPath)
	}

	var found bool
	for _, f := range doc.Findings {
		if f.Subject == "acme.rel" && f.Level == "error" && f.Scope == "config" {
			found = true
		}
		if f.Scope == "" {
			t.Errorf("finding %+v has no scope; every finding is tagged", f)
		}
	}
	if !found {
		t.Errorf("findings = %+v, want the repository reported as an error", doc.Findings)
	}
}

// TestDoctorRejectsFormat proves an unusable -o is refused before any check
// runs, the rule every other -o command follows.
func TestDoctorRejectsFormat(t *testing.T) {
	t.Parallel()

	deps := clitest.New(t, nil)
	err := ExecDoctor("tree", nil, deps.RuntimeCLI)
	if err == nil {
		t.Fatal("ExecDoctor(tree) = nil, want an error")
	}
	if !strings.Contains(err.Error(), "table or json") {
		t.Errorf("error = %v, want it to name the accepted formats", err)
	}
	if deps.Result() != "" {
		t.Errorf("Result Output = %q, want nothing written before the format was rejected", deps.Result())
	}
}
