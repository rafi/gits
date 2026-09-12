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
