package health

// Level is how much a finding matters. It is wire vocabulary — the `-o json`
// document carries these strings — so the three are a contract, and display
// layers map them to color rather than the reverse.
type Level string

const (
	// LevelError is a finding that stops something from working. Any one of
	// them exits non-zero, so a CI job can gate on `gits doctor`.
	LevelError Level = "error"
	// LevelWarning is a finding that degrades gits without stopping it: a
	// missing finder leaves every non-interactive command working.
	LevelWarning Level = "warning"
	// LevelInfo is context rather than a defect — where the cache lives, what
	// git version answered. Reported because the questions this command
	// exists to answer are usually asked with these values in hand.
	LevelInfo Level = "info"
)

// Scope is what a finding is about, so a client can surface the findings its
// users can act on and ignore the rest: a web front-end reports that git is
// missing on the server and has no use for the absence of a finder.
type Scope string

const (
	// ScopeConfig is a defect in the user's config file: an unknown key, a
	// project path that does not resolve, a repository with no local home.
	ScopeConfig Scope = "config"
	// ScopeEnvironment is a defect outside the config: git, the finder, PATH,
	// the cache directory.
	ScopeEnvironment Scope = "environment"
)

// Finding is one thing a health check has to say. Subject names what was
// inspected — a config key, a binary, a project — so findings stay greppable
// and sort into a stable order; Message is the prose.
type Finding struct {
	Level   Level  `json:"level"`
	Scope   Scope  `json:"scope"`
	Subject string `json:"subject"`
	Message string `json:"message"`
}

// HasErrors reports whether any finding is an error, which is what decides a
// caller's exit code.
func HasErrors(findings []Finding) bool {
	for _, f := range findings {
		if f.Level == LevelError {
			return true
		}
	}
	return false
}
