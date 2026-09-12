package doctor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cli/clitest"
	"github.com/rafi/gits/internal/types"
)

// Every test drives ExecDoctor or Check — the command's real entry points —
// with a nil git client, because doctor must never reach one: it inspects
// configuration, the filesystem and PATH, and any git call would be a bug the
// nil client turns into a panic.

// findings returns the findings for the given subject, so a test asserts on
// what was reported rather than on the whole rendered report.
func findings(t *testing.T, deps *clitest.Deps, subject string) []Finding {
	t.Helper()

	var out []Finding
	for _, f := range Check(deps.RuntimeCLI).Findings {
		if f.Subject == subject {
			out = append(out, f)
		}
	}
	return out
}

// TestDoctorReportsUnresolvableRepoHome covers the config defects the loader
// can only report once a project is loaded, and only for the project the user
// happened to name. As configuration problems they had no home before doctor.
func TestDoctorReportsUnresolvableRepoHome(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		project domain.Project
		subject string
		want    string
	}{
		{
			name: "no path and no dir",
			project: domain.Project{
				Repos: []domain.Repository{{Name: "one", Src: "git@example.com:acme/one.git"}},
			},
			subject: "acme.one",
			want:    "nowhere for it to live",
		},
		{
			name: "relative dir without a project path",
			project: domain.Project{
				Repos: []domain.Repository{{Name: "rel", Dir: "relative-dir"}},
			},
			subject: "acme.rel",
			want:    "needs the project to set `path:`",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			deps := clitest.New(t, nil)
			deps.Projects = domain.ProjectListKeyed{"acme": tt.project}

			got := findings(t, deps, tt.subject)
			if len(got) != 1 {
				t.Fatalf("findings for %q = %+v, want exactly one", tt.subject, got)
			}
			if got[0].Level != LevelError {
				t.Errorf("level = %q, want %q", got[0].Level, LevelError)
			}
			if !strings.Contains(got[0].Message, tt.want) {
				t.Errorf("message = %q, want it to contain %q", got[0].Message, tt.want)
			}
		})
	}
}

// TestDoctorExemptsProviderBackedRepos is the counterpart: a provider-backed
// repository legitimately has no local home until it is cloned, so reporting
// one would make every remote project look broken.
func TestDoctorExemptsProviderBackedRepos(t *testing.T) {
	t.Parallel()

	deps := clitest.New(t, nil)
	deps.Projects = domain.ProjectListKeyed{"acme": {
		Source: &domain.ProviderSource{Type: "github", Search: "acme"},
		Repos:  []domain.Repository{{Name: "api", Src: "git@github.com:acme/api.git"}},
	}}

	for _, f := range Check(deps.RuntimeCLI).Findings {
		if f.Level == LevelError {
			t.Errorf("finding %+v, want no error for a provider-backed project", f)
		}
	}
}

// TestDoctorReportsProjectPath covers the `path:` checks: a path that does not
// exist yet is a warning, since `gits clone` creates it, while a path naming a
// file is an error that nothing will fix.
func TestDoctorReportsProjectPath(t *testing.T) {
	t.Parallel()

	t.Run("missing path warns", func(t *testing.T) {
		t.Parallel()

		deps := clitest.New(t, nil)
		deps.Projects = domain.ProjectListKeyed{
			"acme": {Path: filepath.Join(t.TempDir(), "not-created-yet")},
		}

		got := findings(t, deps, "acme")
		if len(got) != 1 || got[0].Level != LevelWarning {
			t.Fatalf("findings = %+v, want one warning", got)
		}
		if !strings.Contains(got[0].Message, "does not exist yet") {
			t.Errorf("message = %q, want it to say the path will be created", got[0].Message)
		}
	})

	t.Run("path naming a file errors", func(t *testing.T) {
		t.Parallel()

		file := filepath.Join(t.TempDir(), "a-file")
		if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
		deps := clitest.New(t, nil)
		deps.Projects = domain.ProjectListKeyed{"acme": {Path: file}}

		got := findings(t, deps, "acme")
		if len(got) != 1 || got[0].Level != LevelError {
			t.Fatalf("findings = %+v, want one error", got)
		}
		if !strings.Contains(got[0].Message, "not a directory") {
			t.Errorf("message = %q, want it to say the path is not a directory", got[0].Message)
		}
	})

	t.Run("existing path is silent", func(t *testing.T) {
		t.Parallel()

		deps := clitest.New(t, nil)
		deps.Projects = domain.ProjectListKeyed{"acme": {Path: t.TempDir()}}

		if got := findings(t, deps, "acme"); len(got) != 0 {
			t.Errorf("findings = %+v, want none for a path that exists", got)
		}
	})
}

// TestDoctorNamesSubProjects proves a finding names its project's path through
// the tree, so `acme/tools` is distinguishable from another project's `tools`.
func TestDoctorNamesSubProjects(t *testing.T) {
	t.Parallel()

	deps := clitest.New(t, nil)
	deps.Projects = domain.ProjectListKeyed{"acme": {
		Path: t.TempDir(),
		SubProjects: []domain.Project{{
			Name: "tools",
			Repos: []domain.Repository{
				{Name: "rel", Dir: "relative-dir"},
			},
		}},
	}}

	// The sub-project inherits its parent's path, which exists, so the
	// repository's relative dir resolves and nothing is reported.
	for _, f := range Check(deps.RuntimeCLI).Findings {
		if f.Level == LevelError {
			t.Errorf("finding %+v, want none: the sub-project inherits a usable path", f)
		}
	}

	// With no parent path to inherit, the same repository is reported, named
	// through the tree.
	deps.Projects = domain.ProjectListKeyed{"acme": {
		SubProjects: []domain.Project{{
			Name:  "tools",
			Repos: []domain.Repository{{Name: "rel", Dir: "relative-dir"}},
		}},
	}}
	got := findings(t, deps, "acme/tools.rel")
	if len(got) != 1 {
		t.Fatalf("findings = %+v, want one naming the sub-project's path through the tree",
			Check(deps.RuntimeCLI).Findings)
	}
}

// TestDoctorReportsConfigWarnings proves the load-time warnings — unknown keys
// among them — reach the report. They are already shown on every run; doctor
// repeats them because it is the one command someone runs wanting everything
// at once.
func TestDoctorReportsConfigWarnings(t *testing.T) {
	t.Parallel()

	deps := clitest.New(t, nil)
	deps.ConfigPath = "/home/nobody/.gits.yaml"
	deps.ConfigWarnings = []string{`unknown config key "acme.pth" in config.yaml, ignored`}

	got := findings(t, deps, "config")
	if len(got) != 2 {
		t.Fatalf("findings = %+v, want the config path and the warning", got)
	}
	if got[1].Level != LevelError || !strings.Contains(got[1].Message, "acme.pth") {
		t.Errorf("finding = %+v, want the unknown key reported as an error", got[1])
	}
}

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
		if !types.IsSilent(err) {
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

// TestDoctorJSONDocument pins the wire shape: one newline-terminated line, so
// it pipes into jq without a reader knowing how many lines to expect.
func TestDoctorJSONDocument(t *testing.T) {
	t.Parallel()

	deps := clitest.New(t, nil)
	deps.ConfigPath = "/home/nobody/.gits.yaml"
	deps.Projects = domain.ProjectListKeyed{"acme": {
		Repos: []domain.Repository{{Name: "rel", Dir: "relative-dir"}},
	}}

	if err := ExecDoctor("json", nil, deps.RuntimeCLI); err != nil && !types.IsSilent(err) {
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
		if f.Subject == "acme.rel" && f.Level == "error" {
			found = true
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

// TestDoctorReportsMissingConfig covers the case with no config file at all:
// every command runs against an empty configuration, which is legitimate but
// worth saying out loud when someone is asking what is wrong.
func TestDoctorReportsMissingConfig(t *testing.T) {
	t.Parallel()

	deps := clitest.New(t, nil)
	deps.ConfigPath = ""

	got := findings(t, deps, "config")
	if len(got) != 1 || got[0].Level != LevelWarning {
		t.Fatalf("findings = %+v, want one warning about the missing config", got)
	}
	if !strings.Contains(got[0].Message, "no config file") {
		t.Errorf("message = %q, want it to say no config file was found", got[0].Message)
	}
}

// TestDoctorReportsCache covers the cache checks, which answer "why is gits
// still showing a repository I deleted last week" — otherwise invisible.
//
// Not parallel: the entries subtest sets XDG_CACHE_HOME, which is
// process-wide, and t.Setenv refuses to run under a parallel parent.
//
//nolint:paralleltest // see above: t.Setenv forbids a parallel parent.
func TestDoctorReportsCache(t *testing.T) {
	t.Run("disabled cache says so and reads no directory", func(t *testing.T) {
		disabled := false
		deps := clitest.New(t, nil)
		deps.Settings.Cache = &disabled

		got := findings(t, deps, "cache")
		if len(got) != 1 {
			t.Fatalf("findings = %+v, want exactly one", got)
		}
		if !strings.Contains(got[0].Message, "disabled") {
			t.Errorf("message = %q, want it to say the cache is disabled", got[0].Message)
		}
	})

	t.Run("entries are described against cacheTTL", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("XDG_CACHE_HOME", dir)
		gitsDir := filepath.Join(dir, "gits")
		if err := os.MkdirAll(gitsDir, 0o750); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		// A file no version of gits will read again: the report exists to say
		// so, since nothing else ever will.
		stale := `{"version":"v0.0","timestamp":"2020-01-01T00:00:00Z","checksum":"x","project":{}}`
		if err := os.WriteFile(filepath.Join(gitsDir, "github-acme.json"), []byte(stale), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
		// And one that cannot be parsed at all, which must not report a
		// nonsense age derived from a zero timestamp.
		if err := os.WriteFile(filepath.Join(gitsDir, "github-broken.json"), []byte("{"), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}

		deps := clitest.New(t, nil)

		got := findings(t, deps, "cache.github-acme")
		if len(got) != 1 || got[0].Level != LevelWarning {
			t.Fatalf("findings = %+v, want one warning for the stale entry", got)
		}
		if !strings.Contains(got[0].Message, "refetched") {
			t.Errorf("message = %q, want it to say the entry will be refetched", got[0].Message)
		}

		broken := findings(t, deps, "cache.github-broken")
		if len(broken) != 1 {
			t.Fatalf("findings = %+v, want one for the corrupt entry", broken)
		}
		// time.Since(zero) is a hundred thousand days; reporting it would read
		// as a bug rather than as "this file is unreadable".
		if strings.Contains(broken[0].Message, "d ago") {
			t.Errorf("message = %q, want no age for an entry with no readable timestamp",
				broken[0].Message)
		}
	})
}

// TestDoctorReportsBinaries covers the git and finder checks. Without it,
// deleting either check outright still passed the whole suite — the two were
// verified by hand and by nothing else.
func TestDoctorReportsBinaries(t *testing.T) {
	t.Parallel()

	t.Run("git is reported", func(t *testing.T) {
		t.Parallel()

		deps := clitest.New(t, nil)
		got := findings(t, deps, "git")
		if len(got) != 1 {
			t.Fatalf("findings = %+v, want exactly one for git", got)
		}
		// git is present in any environment that can run this suite, so the
		// finding is informational and names the binary it found.
		if got[0].Level != LevelInfo {
			t.Errorf("level = %q, want %q where git is on PATH", got[0].Level, LevelInfo)
		}
		if !strings.Contains(got[0].Message, "git") {
			t.Errorf("message = %q, want it to name git and its version", got[0].Message)
		}
	})

	t.Run("a missing finder warns and names it", func(t *testing.T) {
		t.Parallel()

		deps := clitest.New(t, nil)
		deps.Settings.Finder.Binary = "gits-finder-that-does-not-exist"

		// The subject says the finding came from the configured override
		// rather than from the default fzf, so a user knows which to fix.
		got := findings(t, deps, "finder (settings.finder.binary)")
		if len(got) != 1 {
			t.Fatalf("findings = %+v, want one naming the configured finder", got)
		}
		if got[0].Level != LevelWarning {
			t.Errorf("level = %q, want %q: a missing finder degrades gits without stopping it",
				got[0].Level, LevelWarning)
		}
		if !strings.Contains(got[0].Message, deps.Settings.Finder.Binary) {
			t.Errorf("message = %q, want it to name the missing binary", got[0].Message)
		}
	})

	t.Run("an unconfigured finder is reported as fzf", func(t *testing.T) {
		t.Parallel()

		deps := clitest.New(t, nil)
		if got := findings(t, deps, "finder"); len(got) != 1 {
			t.Fatalf("findings = %+v, want one under the plain `finder` subject", got)
		}
	})
}

// TestDoctorChecksEverySubject pins the set of subjects a report covers, so a
// check cannot be dropped silently. Deleting one used to pass the whole suite.
func TestDoctorChecksEverySubject(t *testing.T) {
	t.Parallel()

	deps := clitest.New(t, nil)
	deps.ConfigPath = "/home/nobody/.gits.yaml"
	deps.Projects = domain.ProjectListKeyed{"acme": {Path: t.TempDir()}}

	seen := map[string]bool{}
	for _, f := range Check(deps.RuntimeCLI).Findings {
		seen[f.Subject] = true
	}
	// One subject per check that always reports something. The project and
	// unknown-key checks are silent on a healthy config by design, and are
	// covered by their own tests.
	for _, subject := range []string{"config", "git", "finder", "cache"} {
		if !seen[subject] {
			t.Errorf("no finding for %q; subjects seen: %v", subject, seen)
		}
	}
}
