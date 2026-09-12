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
