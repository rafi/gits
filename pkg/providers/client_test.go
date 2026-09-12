package providers

import (
	"testing"
)

func TestNewGitProvider(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		token    string
		wantType string
		wantErr  bool
	}{
		{"github", "github", "tok", "*providers.gitHubProvider", false},
		{"gitlab", "gitlab", "tok", "*providers.gitLabProvider", false},
		{"bitbucket", "bitbucket", "user:pass", "*providers.bitbucketProvider", false},
		{"filesystem", "filesystem", "", "*providers.filesystemProvider", false},
		{"unknown", "svn", "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := NewGitProvider(tt.provider, Options{Token: tt.token})
			if tt.wantErr {
				if err == nil {
					t.Fatalf("NewGitProvider(%q) = nil error, want error", tt.provider)
				}
				return
			}
			if err != nil {
				t.Fatalf("NewGitProvider(%q) unexpected error: %v", tt.provider, err)
			}
			if got := typeName(p); got != tt.wantType {
				t.Errorf("NewGitProvider(%q) type = %s, want %s", tt.provider, got, tt.wantType)
			}
		})
	}
}

// typeName returns the concrete dynamic type name of v without importing reflect
// at call sites; it keeps the table above readable.
func typeName(v any) string {
	switch v.(type) {
	case *gitHubProvider:
		return "*providers.gitHubProvider"
	case *gitLabProvider:
		return "*providers.gitLabProvider"
	case *bitbucketProvider:
		return "*providers.bitbucketProvider"
	case *filesystemProvider:
		return "*providers.filesystemProvider"
	default:
		return "unknown"
	}
}

func TestGetFirstEnvValue(t *testing.T) {
	t.Run("first non-empty wins", func(t *testing.T) {
		t.Setenv("GITS_TEST_A", "")
		t.Setenv("GITS_TEST_B", "second")
		t.Setenv("GITS_TEST_C", "third")
		got := getFirstEnvValue([]string{"GITS_TEST_A", "GITS_TEST_B", "GITS_TEST_C"})
		if got != "second" {
			t.Errorf("getFirstEnvValue = %q, want %q", got, "second")
		}
	})

	t.Run("all empty yields empty", func(t *testing.T) {
		t.Setenv("GITS_TEST_A", "")
		t.Setenv("GITS_TEST_B", "")
		got := getFirstEnvValue([]string{"GITS_TEST_A", "GITS_TEST_B"})
		if got != "" {
			t.Errorf("getFirstEnvValue = %q, want empty", got)
		}
	})
}
