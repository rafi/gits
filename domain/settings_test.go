package domain

import (
	"testing"
	"time"
)

func TestSettingsProviderTimeoutDuration(t *testing.T) {
	const def = 5 * time.Minute
	tests := []struct {
		name    string
		timeout string
		want    time.Duration
	}{
		{"empty falls back to default", "", def},
		{"valid duration parsed", "90s", 90 * time.Second},
		{"invalid falls back to default", "not-a-duration", def},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := Settings{ProviderTimeout: tt.timeout}
			if got := s.ProviderTimeoutDuration(); got != tt.want {
				t.Errorf("ProviderTimeoutDuration() with %q = %v, want %v", tt.timeout, got, tt.want)
			}
		})
	}
}

func TestSettingsCacheTTLDuration(t *testing.T) {
	const def = 7 * 24 * time.Hour
	tests := []struct {
		name string
		ttl  string
		want time.Duration
	}{
		{"empty falls back to default", "", def},
		{"valid duration parsed", "24h", 24 * time.Hour},
		{"valid minutes parsed", "90m", 90 * time.Minute},
		{"invalid falls back to default", "not-a-duration", def},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := Settings{CacheTTL: tt.ttl}
			if got := s.CacheTTLDuration(); got != tt.want {
				t.Errorf("CacheTTLDuration() with %q = %v, want %v", tt.ttl, got, tt.want)
			}
		})
	}
}

func TestSettingsGitTimeoutDuration(t *testing.T) {
	const def = 5 * time.Minute
	tests := []struct {
		name    string
		timeout string
		want    time.Duration
	}{
		{"empty falls back to default", "", def},
		{"valid duration parsed", "30m", 30 * time.Minute},
		{"invalid falls back to default", "not-a-duration", def},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := Settings{GitTimeout: tt.timeout}
			if got := s.GitTimeoutDuration(); got != tt.want {
				t.Errorf("GitTimeoutDuration() with %q = %v, want %v", tt.timeout, got, tt.want)
			}
		})
	}
}

func TestSettingsProviderAuth(t *testing.T) {
	s := Settings{
		GitHub:    ProviderSettings{TokenCommand: "pass tokens/github"},
		GitLab:    ProviderSettings{Token: "gl-token"},
		Bitbucket: ProviderSettings{TokenCmd: "pass tokens/bitbucket"},
	}
	tests := []struct {
		provider string
		want     ProviderSettings
	}{
		{"github", s.GitHub},
		{"gitlab", s.GitLab},
		{"bitbucket", s.Bitbucket},
		{"GitHub", s.GitHub},
		{"filesystem", ProviderSettings{}},
		{"", ProviderSettings{}},
		{"svn", ProviderSettings{}},
	}
	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			if got := s.ProviderAuth(tt.provider); got != tt.want {
				t.Errorf("ProviderAuth(%q) = %+v, want %+v", tt.provider, got, tt.want)
			}
		})
	}
}

func TestProviderSettingsCommand(t *testing.T) {
	tests := []struct {
		name     string
		settings ProviderSettings
		want     string
	}{
		{"none configured", ProviderSettings{}, ""},
		{"tokenCommand", ProviderSettings{TokenCommand: "a"}, "a"},
		{"token-cmd alias", ProviderSettings{TokenCmd: "b"}, "b"},
		{"canonical key wins", ProviderSettings{TokenCommand: "a", TokenCmd: "b"}, "a"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.settings.Command(); got != tt.want {
				t.Errorf("Command() = %q, want %q", got, tt.want)
			}
		})
	}
}
