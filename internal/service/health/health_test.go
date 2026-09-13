package health

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/service"
)

// Every test drives Check — the real entry point — with a nil git client,
// because health must never reach one: it inspects configuration, the
// filesystem and PATH, and any git call would be a bug the nil client turns
// into a panic. Nothing here writes output either; the checks run headless.

// runtime builds the business runtime the checks read from.
func runtime(t *testing.T) service.Runtime {
	t.Helper()

	settings := domain.Settings{WorkerCount: 1}
	settings.Icons.ApplyDefaults()
	return service.Runtime{
		Ctx:      t.Context(),
		Settings: settings,
		Projects: domain.ProjectListKeyed{},
	}
}

// findings returns the findings for the given subject, so a test asserts on
// what was reported rather than on the whole rendered report.
func findings(t *testing.T, rt service.Runtime, subject string) []Finding {
	t.Helper()

	var out []Finding
	for _, f := range Check(rt) {
		if f.Subject == subject {
			out = append(out, f)
		}
	}
	return out
}

// TestReportsUnresolvableRepoHome covers the config defects the loader can
// only report once a project is loaded, and only for the project the user
// happened to name. As configuration problems they had no home before doctor.
func TestReportsUnresolvableRepoHome(t *testing.T) {
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

			rt := runtime(t)
			rt.Projects = domain.ProjectListKeyed{"acme": tt.project}

			got := findings(t, rt, tt.subject)
			if len(got) != 1 {
				t.Fatalf("findings for %q = %+v, want exactly one", tt.subject, got)
			}
			if got[0].Level != LevelError {
				t.Errorf("level = %q, want %q", got[0].Level, LevelError)
			}
			if got[0].Scope != ScopeConfig {
				t.Errorf("scope = %q, want %q", got[0].Scope, ScopeConfig)
			}
			if !strings.Contains(got[0].Message, tt.want) {
				t.Errorf("message = %q, want it to contain %q", got[0].Message, tt.want)
			}
		})
	}
}

// TestExemptsProviderBackedRepos is the counterpart: a provider-backed
// repository legitimately has no local home until it is cloned, so reporting
// one would make every remote project look broken.
func TestExemptsProviderBackedRepos(t *testing.T) {
	t.Parallel()

	rt := runtime(t)
	rt.Projects = domain.ProjectListKeyed{"acme": {
		Source: &domain.ProviderSource{Type: "github", Search: "acme"},
		Repos:  []domain.Repository{{Name: "api", Src: "git@github.com:acme/api.git"}},
	}}

	for _, f := range Check(rt) {
		if f.Level == LevelError {
			t.Errorf("finding %+v, want no error for a provider-backed project", f)
		}
	}
}

// TestReportsProjectPath covers the `path:` checks: a path that does not exist
// yet is a warning, since `gits clone` creates it, while a path naming a file
// is an error that nothing will fix.
func TestReportsProjectPath(t *testing.T) {
	t.Parallel()

	t.Run("missing path warns", func(t *testing.T) {
		t.Parallel()

		rt := runtime(t)
		rt.Projects = domain.ProjectListKeyed{
			"acme": {Path: filepath.Join(t.TempDir(), "not-created-yet")},
		}

		got := findings(t, rt, "acme")
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
		rt := runtime(t)
		rt.Projects = domain.ProjectListKeyed{"acme": {Path: file}}

		got := findings(t, rt, "acme")
		if len(got) != 1 || got[0].Level != LevelError {
			t.Fatalf("findings = %+v, want one error", got)
		}
		if !strings.Contains(got[0].Message, "not a directory") {
			t.Errorf("message = %q, want it to say the path is not a directory", got[0].Message)
		}
	})

	t.Run("existing path is silent", func(t *testing.T) {
		t.Parallel()

		rt := runtime(t)
		rt.Projects = domain.ProjectListKeyed{"acme": {Path: t.TempDir()}}

		if got := findings(t, rt, "acme"); len(got) != 0 {
			t.Errorf("findings = %+v, want none for a path that exists", got)
		}
	})
}

// TestNamesSubProjects proves a finding names its project's path through the
// tree, so `acme/tools` is distinguishable from another project's `tools`.
func TestNamesSubProjects(t *testing.T) {
	t.Parallel()

	rt := runtime(t)
	rt.Projects = domain.ProjectListKeyed{"acme": {
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
	for _, f := range Check(rt) {
		if f.Level == LevelError {
			t.Errorf("finding %+v, want none: the sub-project inherits a usable path", f)
		}
	}

	// With no parent path to inherit, the same repository is reported, named
	// through the tree.
	rt.Projects = domain.ProjectListKeyed{"acme": {
		SubProjects: []domain.Project{{
			Name:  "tools",
			Repos: []domain.Repository{{Name: "rel", Dir: "relative-dir"}},
		}},
	}}
	got := findings(t, rt, "acme/tools.rel")
	if len(got) != 1 {
		t.Fatalf("findings = %+v, want one naming the sub-project's path through the tree",
			Check(rt))
	}
}

// TestReportsConfigWarnings proves the load-time warnings — unknown keys among
// them — reach the report. They are already shown on every run; doctor repeats
// them because it is the one command someone runs wanting everything at once.
func TestReportsConfigWarnings(t *testing.T) {
	t.Parallel()

	rt := runtime(t)
	rt.ConfigPath = "/home/nobody/.gits.yaml"
	rt.ConfigWarnings = []string{`unknown config key "acme.pth" in config.yaml, ignored`}

	got := findings(t, rt, "config")
	if len(got) != 2 {
		t.Fatalf("findings = %+v, want the config path and the warning", got)
	}
	if got[1].Level != LevelError || !strings.Contains(got[1].Message, "acme.pth") {
		t.Errorf("finding = %+v, want the unknown key reported as an error", got[1])
	}
}

// TestHasErrors pins the rule tickets 01 and 02 established: an error finding
// decides a non-zero exit so CI can gate on it, and warnings and info do not.
func TestHasErrors(t *testing.T) {
	t.Parallel()

	t.Run("an error finding fails the run", func(t *testing.T) {
		t.Parallel()

		rt := runtime(t)
		rt.Projects = domain.ProjectListKeyed{"acme": {
			Repos: []domain.Repository{{Name: "rel", Dir: "relative-dir"}},
		}}

		if !HasErrors(Check(rt)) {
			t.Error("HasErrors = false, want true for an error finding")
		}
	})

	t.Run("warnings alone succeed", func(t *testing.T) {
		t.Parallel()

		rt := runtime(t)
		rt.Projects = domain.ProjectListKeyed{
			"acme": {Path: filepath.Join(t.TempDir(), "not-created-yet")},
		}

		if HasErrors(Check(rt)) {
			t.Error("HasErrors = true, want false: a warning is not a failure")
		}
	})
}

// TestReportsMissingConfig covers the case with no config file at all: every
// command runs against an empty configuration, which is legitimate but worth
// saying out loud when someone is asking what is wrong.
func TestReportsMissingConfig(t *testing.T) {
	t.Parallel()

	rt := runtime(t)
	rt.ConfigPath = ""

	got := findings(t, rt, "config")
	if len(got) != 1 || got[0].Level != LevelWarning {
		t.Fatalf("findings = %+v, want one warning about the missing config", got)
	}
	if !strings.Contains(got[0].Message, "no config file") {
		t.Errorf("message = %q, want it to say no config file was found", got[0].Message)
	}
}

// TestReportsCache covers the cache checks, which answer "why is gits still
// showing a repository I deleted last week" — otherwise invisible.
//
// Not parallel: the entries subtest sets XDG_CACHE_HOME, which is
// process-wide, and t.Setenv refuses to run under a parallel parent.
//
//nolint:paralleltest // see above: t.Setenv forbids a parallel parent.
func TestReportsCache(t *testing.T) {
	t.Run("disabled cache says so and reads no directory", func(t *testing.T) {
		disabled := false
		rt := runtime(t)
		rt.Settings.Cache = &disabled

		got := findings(t, rt, "cache")
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

		rt := runtime(t)

		got := findings(t, rt, "cache.github-acme")
		if len(got) != 1 || got[0].Level != LevelWarning {
			t.Fatalf("findings = %+v, want one warning for the stale entry", got)
		}
		if !strings.Contains(got[0].Message, "refetched") {
			t.Errorf("message = %q, want it to say the entry will be refetched", got[0].Message)
		}

		broken := findings(t, rt, "cache.github-broken")
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

// TestReportsBinaries covers the git and finder checks. Without it, deleting
// either check outright still passed the whole suite — the two were verified
// by hand and by nothing else.
func TestReportsBinaries(t *testing.T) {
	t.Parallel()

	t.Run("git is reported", func(t *testing.T) {
		t.Parallel()

		got := findings(t, runtime(t), "git")
		if len(got) != 1 {
			t.Fatalf("findings = %+v, want exactly one for git", got)
		}
		// git is present in any environment that can run this suite, so the
		// finding is informational and names the binary it found.
		if got[0].Level != LevelInfo {
			t.Errorf("level = %q, want %q where git is on PATH", got[0].Level, LevelInfo)
		}
		if got[0].Scope != ScopeEnvironment {
			t.Errorf("scope = %q, want %q", got[0].Scope, ScopeEnvironment)
		}
		if !strings.Contains(got[0].Message, "git") {
			t.Errorf("message = %q, want it to name git and its version", got[0].Message)
		}
	})

	t.Run("a missing finder warns and names it", func(t *testing.T) {
		t.Parallel()

		rt := runtime(t)
		rt.Settings.Finder.Binary = "gits-finder-that-does-not-exist"

		// The subject says the finding came from the configured override
		// rather than from the default fzf, so a user knows which to fix.
		got := findings(t, rt, "finder (settings.finder.binary)")
		if len(got) != 1 {
			t.Fatalf("findings = %+v, want one naming the configured finder", got)
		}
		if got[0].Level != LevelWarning {
			t.Errorf("level = %q, want %q: a missing finder degrades gits without stopping it",
				got[0].Level, LevelWarning)
		}
		if !strings.Contains(got[0].Message, rt.Settings.Finder.Binary) {
			t.Errorf("message = %q, want it to name the missing binary", got[0].Message)
		}
	})

	t.Run("an unconfigured finder is reported as fzf", func(t *testing.T) {
		t.Parallel()

		if got := findings(t, runtime(t), "finder"); len(got) != 1 {
			t.Fatalf("findings = %+v, want one under the plain `finder` subject", got)
		}
	})
}

// TestChecksEverySubject pins the set of subjects a report covers, so a check
// cannot be dropped silently. Deleting one used to pass the whole suite.
func TestChecksEverySubject(t *testing.T) {
	t.Parallel()

	rt := runtime(t)
	rt.ConfigPath = "/home/nobody/.gits.yaml"
	rt.Projects = domain.ProjectListKeyed{"acme": {Path: t.TempDir()}}

	seen := map[string]bool{}
	for _, f := range Check(rt) {
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
