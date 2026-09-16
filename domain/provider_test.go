package domain

import (
	"strings"
	"testing"
)

func TestProviderSourceUniqueKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ps   ProviderSource
		want string
	}{
		{"no slash", ProviderSource{Type: "github", Search: "rafi"}, "github-rafi"},
		{"slash escaped", ProviderSource{Type: "gitlab", Search: "group/sub"}, "gitlab-group%sub"},
		{"multiple slashes", ProviderSource{Type: "gitlab", Search: "a/b/c"}, "gitlab-a%b%c"},
		{"empty search", ProviderSource{Type: "github", Search: ""}, "github-"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.ps.UniqueKey(); got != tt.want {
				t.Errorf("UniqueKey() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestProviderSourceValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		ps        ProviderSource
		wantErr   bool
		errSubstr string
	}{
		{"github ok", ProviderSource{Type: "github", Search: "rafi"}, false, ""},
		{"gitlab ok", ProviderSource{Type: "gitlab", Search: "42"}, false, ""},
		{"bitbucket ok", ProviderSource{Type: "bitbucket", Search: "team"}, false, ""},
		{"filesystem ok", ProviderSource{Type: "filesystem", Search: "/code"}, false, ""},
		{"unknown type", ProviderSource{Type: "svn", Search: "x"}, true, "unknown source type"},
		{"github empty search names owner", ProviderSource{Type: "github", Search: ""}, true, "owner"},
		{"gitlab empty search names groupID", ProviderSource{Type: "gitlab", Search: ""}, true, "groupID"},
		{"bitbucket empty search names owner", ProviderSource{Type: "bitbucket", Search: ""}, true, "owner"},
		{"gitea empty search names owner", ProviderSource{Type: "gitea", Search: "", URL: "https://gitea.com"}, true, "owner"},
		{"forgejo ok", ProviderSource{Type: "forgejo", Search: "rafi", URL: "https://codeberg.org"}, false, ""},
		{"filesystem empty search names path", ProviderSource{Type: "filesystem", Search: ""}, true, "path"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.ps.Validate()
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Validate() = nil, want error")
				}
				if tt.errSubstr != "" && !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("Validate() error = %q, want substring %q", err, tt.errSubstr)
				}
				return
			}
			if err != nil {
				t.Errorf("Validate() = %v, want nil", err)
			}
		})
	}
}

func TestProviderSourceValidateURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		ps        ProviderSource
		errSubstr string // empty: valid
	}{
		{"github host", ProviderSource{Type: "github", Search: "acme", URL: "https://github.corp.example"}, ""},
		{"gitlab host with path", ProviderSource{Type: "gitlab", Search: "42", URL: "https://host/gitlab/"}, ""},
		{"user accepted", ProviderSource{Type: "github", Search: "acme", URL: "https://rafi@git.corp.com"}, ""},
		{"public host", ProviderSource{Type: "github", Search: "acme", URL: "https://github.com"}, ""},
		{"gitea requires url", ProviderSource{Type: "gitea", Search: "acme"}, "gitea"},
		{"forgejo requires url", ProviderSource{Type: "forgejo", Search: "rafi"}, "forgejo"},
		{"gitea host", ProviderSource{Type: "gitea", Search: "acme", URL: "https://gitea.com/"}, ""},
		{"bitbucket rejects url", ProviderSource{Type: "bitbucket", Search: "team", URL: "https://bb.corp"}, "bitbucket"},
		{"filesystem rejects url", ProviderSource{Type: "filesystem", Search: "/code", URL: "https://x"}, "filesystem"},
		{"password rejected", ProviderSource{Type: "github", Search: "acme", URL: "https://rafi:s3cret@git.corp.com"}, "password"},
		{"no scheme", ProviderSource{Type: "github", Search: "acme", URL: "git.corp.com"}, "url"},
		{"non-http scheme", ProviderSource{Type: "github", Search: "acme", URL: "ssh://git.corp.com"}, "url"},
		{"query", ProviderSource{Type: "github", Search: "acme", URL: "https://git.corp.com/?a=b"}, "url"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.ps.Validate()
			if tt.errSubstr == "" {
				if err != nil {
					t.Errorf("Validate() = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate() = nil, want error naming %q", tt.errSubstr)
			}
			if !strings.Contains(err.Error(), tt.errSubstr) {
				t.Errorf("Validate() error = %q, want substring %q", err, tt.errSubstr)
			}
			if strings.Contains(err.Error(), "s3cret") {
				t.Errorf("Validate() error leaks the password: %q", err)
			}
		})
	}
}

func TestProviderSourceBaseURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ps   ProviderSource
		want string
	}{
		{"unset", ProviderSource{Type: "github"}, ""},
		{"custom host", ProviderSource{Type: "github", URL: "https://git.corp.com"}, "https://git.corp.com"},
		{"trailing slash", ProviderSource{Type: "gitlab", URL: "https://host/gitlab/"}, "https://host/gitlab"},
		{"host case", ProviderSource{Type: "gitlab", URL: "HTTPS://Git.Corp.com"}, "https://git.corp.com"},
		{"user dropped", ProviderSource{Type: "github", URL: "https://rafi@git.corp.com:8443"}, "https://git.corp.com:8443"},
		{"github public", ProviderSource{Type: "github", URL: "https://github.com/"}, ""},
		{"gitlab public", ProviderSource{Type: "gitlab", URL: "https://gitlab.com"}, ""},
		{"public host of another type", ProviderSource{Type: "gitlab", URL: "https://github.com"}, "https://github.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.ps.BaseURL(); got != tt.want {
				t.Errorf("BaseURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestProviderSourceURLUser(t *testing.T) {
	t.Parallel()

	if got := (ProviderSource{Type: "github", URL: "https://rafi@git.corp.com"}).URLUser(); got != "rafi" {
		t.Errorf("URLUser() = %q, want %q", got, "rafi")
	}
	if got := (ProviderSource{Type: "github", URL: "https://git.corp.com"}).URLUser(); got != "" {
		t.Errorf("URLUser() = %q, want empty", got)
	}
}

func TestProviderSourceUniqueKeyURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ps   ProviderSource
		want string
	}{
		{"public host keys as unset", ProviderSource{Type: "github", Search: "acme", URL: "https://github.com"}, "github-acme"},
		{"custom host", ProviderSource{Type: "github", Search: "acme", URL: "https://git.corp.com/"}, "github-git.corp.com-acme"},
		{"port and path", ProviderSource{Type: "gitlab", Search: "a/b", URL: "https://host:8443/gl"}, "gitlab-host_8443%gl-a%b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.ps.UniqueKey(); got != tt.want {
				t.Errorf("UniqueKey() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestUniqueKeySeparatesURLUsers checks that a URL user changes the key
// without appearing in it.
func TestUniqueKeySeparatesURLUsers(t *testing.T) {
	t.Parallel()

	plain := ProviderSource{Type: "github", Search: "acme", URL: "https://git.corp.com"}
	rafi := ProviderSource{Type: "github", Search: "acme", URL: "https://rafi@git.corp.com"}
	other := ProviderSource{Type: "github", Search: "acme", URL: "https://other@git.corp.com"}

	if rafi.UniqueKey() == plain.UniqueKey() || rafi.UniqueKey() == other.UniqueKey() {
		t.Errorf("URL users share a cache entry: %q, %q, %q",
			plain.UniqueKey(), rafi.UniqueKey(), other.UniqueKey())
	}
	if strings.Contains(rafi.UniqueKey(), "rafi") {
		t.Errorf("UniqueKey() leaks the URL user: %q", rafi.UniqueKey())
	}
}
