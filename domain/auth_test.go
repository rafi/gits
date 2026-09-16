package domain

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestSettingsSourceAuth checks that source credentials override settings.
func TestSettingsSourceAuth(t *testing.T) {
	t.Parallel()

	settings := Settings{
		GitHub: ProviderSettings{Token: "settings-gh", TokenCommand: "settings-gh-cmd"},
		GitLab: ProviderSettings{Token: "settings-gl"},
	}
	tests := []struct {
		name   string
		source *ProviderSource
		want   ProviderSettings
	}{
		{
			name:   "source token wins over settings",
			source: &ProviderSource{Type: "github", Token: "source-gh"},
			want:   ProviderSettings{Token: "source-gh"},
		},
		{
			// A source command is not outranked by a settings token.
			name:   "source command wins over a settings token",
			source: &ProviderSource{Type: "github", TokenCommand: "source-cmd"},
			want:   ProviderSettings{TokenCommand: "source-cmd"},
		},
		{
			name:   "token-cmd alias counts as declared",
			source: &ProviderSource{Type: "github", TokenCmd: "source-alias"},
			want:   ProviderSettings{TokenCmd: "source-alias"},
		},
		{
			name:   "no source credentials falls back to settings",
			source: &ProviderSource{Type: "github"},
			want:   settings.GitHub,
		},
		{
			name:   "fallback is per provider",
			source: &ProviderSource{Type: "gitlab"},
			want:   settings.GitLab,
		},
		{
			name:   "filesystem authenticates against nothing",
			source: &ProviderSource{Type: "filesystem", Search: "~/code"},
			want:   ProviderSettings{},
		},
		{
			name:   "no source at all",
			source: nil,
			want:   ProviderSettings{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := settings.SourceAuth(tt.source); got != tt.want {
				t.Errorf("SourceAuth() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// TestProviderSourceWithoutAuth checks that only credentials are removed.
func TestProviderSourceWithoutAuth(t *testing.T) {
	t.Parallel()

	source := ProviderSource{
		Type:         "github",
		Search:       "acme",
		Token:        "tok",
		TokenCommand: "cmd",
		TokenCmd:     "alias",
	}
	got := source.WithoutAuth()
	if got.Type != "github" || got.Search != "acme" {
		t.Errorf("WithoutAuth() lost the source identity: %+v", got)
	}
	if !got.Auth().IsZero() {
		t.Errorf("WithoutAuth() kept credentials: %+v", got)
	}
	// The original must be untouched.
	if source.Token != "tok" {
		t.Error("WithoutAuth() mutated its receiver")
	}
}

// TestProjectWithoutAuthRedactsSubProjects checks that redaction covers the
// whole tree without mutating the original.
func TestProjectWithoutAuthRedactsSubProjects(t *testing.T) {
	t.Parallel()

	project := Project{
		Name:   "acme",
		Source: &ProviderSource{Type: "github", Search: "acme", Token: "parent-tok"},
		SubProjects: []Project{{
			Name:   "tools",
			Source: &ProviderSource{Type: "gitlab", Search: "42", TokenCommand: "sub-cmd"},
		}},
	}

	got := project.WithoutAuth()
	if !got.Source.Auth().IsZero() {
		t.Errorf("parent source kept credentials: %+v", got.Source)
	}
	if !got.SubProjects[0].Source.Auth().IsZero() {
		t.Errorf("sub-project source kept credentials: %+v", got.SubProjects[0].Source)
	}
	if project.Source.Token != "parent-tok" {
		t.Error("redaction reached back into the caller's parent source")
	}
	if project.SubProjects[0].Source.TokenCommand != "sub-cmd" {
		t.Error("redaction reached back into the caller's sub-project source")
	}
}

// TestProjectMarshalRedacted checks that redacted JSON holds no credentials.
func TestProjectMarshalRedacted(t *testing.T) {
	t.Parallel()

	project := Project{
		Name:   "acme",
		Source: &ProviderSource{Type: "github", Search: "acme", Token: "s3cret"},
		SubProjects: []Project{{
			Name:   "tools",
			Source: &ProviderSource{Type: "github", Search: "tools", TokenCmd: "pass work"},
		}},
	}
	raw, err := json.Marshal(project.WithoutAuth())
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	for _, secret := range []string{"s3cret", "pass work", "token"} {
		if strings.Contains(string(raw), secret) {
			t.Errorf("marshaled project contains %q: %s", secret, raw)
		}
	}
}

// TestProjectIdentityRedactsSource checks that Identity omits credentials.
func TestProjectIdentityRedactsSource(t *testing.T) {
	t.Parallel()

	project := Project{
		Name:   "acme",
		Source: &ProviderSource{Type: "github", Search: "acme", Token: "s3cret"},
	}
	identity := project.Identity()
	if identity.Source.Token != "" {
		t.Errorf("Identity() carried a token: %+v", identity.Source)
	}
	if identity.Source.Type != "github" || identity.Source.Search != "acme" {
		t.Errorf("Identity() lost the source identity: %+v", identity.Source)
	}
	if project.Source.Token != "s3cret" {
		t.Error("Identity() cleared the credential the loader needs")
	}
}

// TestCalculateHashIgnoresCredentials checks that the hash ignores credentials.
func TestCalculateHashIgnoresCredentials(t *testing.T) {
	t.Parallel()

	hashOf := func(t *testing.T, source *ProviderSource) string {
		t.Helper()
		project := Project{Name: "acme", Source: source}
		if err := project.CalculateHash(); err != nil {
			t.Fatalf("CalculateHash() error = %v", err)
		}
		return project.Hash
	}

	base := hashOf(t, &ProviderSource{Type: "github", Search: "acme"})
	withToken := hashOf(t, &ProviderSource{Type: "github", Search: "acme", Token: "tok"})
	if base != withToken {
		t.Errorf("hash changed with a credential: %q vs %q", base, withToken)
	}

	// The search still affects the hash.
	other := hashOf(t, &ProviderSource{Type: "github", Search: "other"})
	if base == other {
		t.Error("hash ignored the search term")
	}
}

// TestUniqueKeySeparatesCredentials checks that different credentials get
// different keys.
func TestUniqueKeySeparatesCredentials(t *testing.T) {
	t.Parallel()

	plain := ProviderSource{Type: "github", Search: "acme"}
	work := ProviderSource{Type: "github", Search: "acme", Token: "work"}
	personal := ProviderSource{Type: "github", Search: "acme", Token: "personal"}

	// No credentials: the key is unchanged.
	if got := plain.UniqueKey(); got != "github-acme" {
		t.Errorf("UniqueKey() = %q, want %q", got, "github-acme")
	}
	if work.UniqueKey() == personal.UniqueKey() {
		t.Errorf("two credentials share a cache entry: %q", work.UniqueKey())
	}
	if work.UniqueKey() == plain.UniqueKey() {
		t.Errorf("a credentialed source shares the settings entry: %q", work.UniqueKey())
	}
	// Same credential, same key.
	if work.UniqueKey() != (ProviderSource{Type: "github", Search: "acme", Token: "work"}).UniqueKey() {
		t.Error("UniqueKey() is not stable for one credential")
	}
	// The key must not contain the credential.
	if strings.Contains(work.UniqueKey(), "work") {
		t.Errorf("UniqueKey() leaks the credential: %q", work.UniqueKey())
	}
}

// TestRestoreAuth checks that credentials are restored onto a cached project.
func TestRestoreAuth(t *testing.T) {
	t.Parallel()

	configured := Project{
		Name:   "acme",
		Source: &ProviderSource{Type: "github", Search: "acme", Token: "tok"},
		SubProjects: []Project{{
			Name:   "tools",
			Source: &ProviderSource{Type: "gitlab", Search: "42", TokenCommand: "cmd"},
		}},
	}
	cached := configured.WithoutAuth()

	cached.RestoreAuth(configured)
	if cached.Source.Token != "tok" {
		t.Errorf("parent credential not restored: %+v", cached.Source)
	}
	if cached.SubProjects[0].Source.TokenCommand != "cmd" {
		t.Errorf("sub-project credential not restored: %+v", cached.SubProjects[0].Source)
	}
}

// TestRestoreAuthWithProviderSubProjects checks that sub-projects absent from
// the config are left alone.
func TestRestoreAuthWithProviderSubProjects(t *testing.T) {
	t.Parallel()

	configured := Project{
		Name:   "acme",
		Source: &ProviderSource{Type: "gitlab", Search: "42", Token: "tok"},
	}
	cached := Project{
		Name:   "acme",
		Source: &ProviderSource{Type: "gitlab", Search: "42"},
		SubProjects: []Project{{
			Name:   "discovered",
			Source: &ProviderSource{Type: "gitlab", Search: "43"},
		}},
	}

	cached.RestoreAuth(configured)
	if cached.Source.Token != "tok" {
		t.Errorf("parent credential not restored: %+v", cached.Source)
	}
	if !cached.SubProjects[0].Source.Auth().IsZero() {
		t.Errorf("discovered sub-project given a credential: %+v", cached.SubProjects[0].Source)
	}
}

// TestWithoutAuthStripsURLUser checks that the URL keeps its host but loses
// its user, and that RestoreAuth puts the user back.
func TestWithoutAuthStripsURLUser(t *testing.T) {
	t.Parallel()

	configured := Project{
		Name:   "work",
		Source: &ProviderSource{Type: "github", Search: "acme", URL: "https://rafi@git.corp.com/"},
	}
	cached := configured.WithoutAuth()
	if got := cached.Source.URL; got != "https://git.corp.com/" {
		t.Errorf("WithoutAuth() URL = %q, want %q", got, "https://git.corp.com/")
	}
	if configured.Source.URL != "https://rafi@git.corp.com/" {
		t.Error("WithoutAuth() mutated its receiver")
	}

	raw, err := json.Marshal(cached)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if strings.Contains(string(raw), "rafi") {
		t.Errorf("marshaled project contains the URL user: %s", raw)
	}
	if identity := configured.Identity(); strings.Contains(identity.Source.URL, "rafi") {
		t.Errorf("Identity() carried the URL user: %q", identity.Source.URL)
	}

	cached.RestoreAuth(configured)
	if got := cached.Source.URL; got != "https://rafi@git.corp.com/" {
		t.Errorf("RestoreAuth() URL = %q, want the configured one", got)
	}
}

// TestWithoutAuthDropsUnparseableURL checks that a URL too malformed to strip
// is removed rather than written out with whatever it holds.
func TestWithoutAuthDropsUnparseableURL(t *testing.T) {
	t.Parallel()

	source := ProviderSource{Type: "github", Search: "acme", URL: "https://rafi:s3cret@host/%zz"}
	if got := source.WithoutAuth().URL; got != "" {
		t.Errorf("WithoutAuth() URL = %q, want empty", got)
	}
}
