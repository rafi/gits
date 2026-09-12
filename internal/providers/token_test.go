package providers

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// runtimeIsWindows reports whether the shell fixtures below (sh syntax) can
// run on this platform.
func runtimeIsWindows() bool { return runtime.GOOS == "windows" }

// resetTokenCache clears memoized token command output so cases don't leak
// into one another.
func resetTokenCache(t *testing.T) {
	t.Helper()
	tokenCacheMu.Lock()
	tokenCache = map[string]string{}
	tokenCacheMu.Unlock()
	t.Cleanup(func() {
		tokenCacheMu.Lock()
		tokenCache = map[string]string{}
		tokenCacheMu.Unlock()
	})
}

// clearTokenEnv unsets every provider token environment variable, so the env
// fallback can't mask what a case is proving.
func clearTokenEnv(t *testing.T) {
	t.Helper()
	for _, names := range tokenEnvVarNames {
		for _, name := range names {
			t.Setenv(name, "")
		}
	}
}

//nolint:paralleltest // resetTokenCache clears the package-level token cache, which every case here needs to itself.
func TestRunTokenCommand(t *testing.T) {
	if runtimeIsWindows() {
		t.Skip("shell fixtures assume a POSIX shell")
	}
	tests := []struct {
		name    string
		command string
		want    string
		wantErr bool
	}{
		{"single line", "echo tok", "tok", false},
		{"trailing whitespace trimmed", "printf '  tok  \\n'", "tok", false},
		{"first non-empty line wins", "printf '\\n\\ntok\\nnotes\\n'", "tok", false},
		{"pipes are honored", "printf 'a\\nb\\n' | tail -n1", "b", false},
		{"no output is an error", "true", "", true},
		{"whitespace only is an error", "printf '  \\n'", "", true},
		{"non-zero exit is an error", "exit 3", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetTokenCache(t)
			got, err := runTokenCommand(t.Context(), tt.command)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("runTokenCommand(%q) = %q, want error", tt.command, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("runTokenCommand(%q): %v", tt.command, err)
			}
			if got != tt.want {
				t.Errorf("runTokenCommand(%q) = %q, want %q", tt.command, got, tt.want)
			}
		})
	}
}

// TestRunTokenCommandErrorIncludesStderr proves a failing helper reports why
// it failed, instead of a bare "exit status 1".
//
//nolint:paralleltest // resetTokenCache clears the package-level token cache, which every case here needs to itself.
func TestRunTokenCommandErrorIncludesStderr(t *testing.T) {
	if runtimeIsWindows() {
		t.Skip("shell fixtures assume a POSIX shell")
	}
	resetTokenCache(t)
	_, err := runTokenCommand(t.Context(), "echo 'gpg: decryption failed' >&2; exit 2")
	if err == nil {
		t.Fatal("runTokenCommand(failing) = nil error, want error")
	}
	if !strings.Contains(err.Error(), "decryption failed") {
		t.Errorf("error = %v, want it to include the command's stderr", err)
	}
}

// TestRunTokenCommandCaches proves the command runs once per process, so
// several projects sharing a provider don't trigger repeated passphrase
// prompts.
//
//nolint:paralleltest // resetTokenCache clears the package-level token cache, which every case here needs to itself.
func TestRunTokenCommandCaches(t *testing.T) {
	if runtimeIsWindows() {
		t.Skip("shell fixtures assume a POSIX shell")
	}
	resetTokenCache(t)
	counter := filepath.Join(t.TempDir(), "runs")
	command := "printf x >> " + counter + "; echo tok"

	for range 3 {
		got, err := runTokenCommand(t.Context(), command)
		if err != nil {
			t.Fatalf("runTokenCommand: %v", err)
		}
		if got != "tok" {
			t.Fatalf("runTokenCommand = %q, want %q", got, "tok")
		}
	}
	runs, err := os.ReadFile(counter)
	if err != nil {
		t.Fatalf("read counter: %v", err)
	}
	if len(runs) != 1 {
		t.Errorf("command ran %d times, want 1", len(runs))
	}
}

func TestResolveTokenPrecedence(t *testing.T) {
	if runtimeIsWindows() {
		t.Skip("shell fixtures assume a POSIX shell")
	}
	tests := []struct {
		name string
		env  string
		opts Options
		want string
	}{
		{
			name: "token wins over command and env",
			env:  "from-env",
			opts: Options{Token: "from-config", TokenCommand: "echo from-cmd"},
			want: "from-config",
		},
		{
			name: "command wins over env",
			env:  "from-env",
			opts: Options{TokenCommand: "echo from-cmd"},
			want: "from-cmd",
		},
		{
			name: "env is the last resort",
			env:  "from-env",
			opts: Options{},
			want: "from-env",
		},
		{
			name: "blank command is ignored",
			env:  "from-env",
			opts: Options{TokenCommand: "   "},
			want: "from-env",
		},
		{
			name: "nothing configured yields empty",
			opts: Options{},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetTokenCache(t)
			clearTokenEnv(t)
			t.Setenv("GITHUB_TOKEN", tt.env)
			got, err := resolveToken(t.Context(), ProviderGitHub, tt.opts)
			if err != nil {
				t.Fatalf("resolveToken: %v", err)
			}
			if got != tt.want {
				t.Errorf("resolveToken = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestResolveTokenCommandFailureIsFatal proves a broken token command is
// reported rather than silently falling back to the environment, which would
// hide the misconfiguration.
func TestResolveTokenCommandFailureIsFatal(t *testing.T) {
	if runtimeIsWindows() {
		t.Skip("shell fixtures assume a POSIX shell")
	}
	resetTokenCache(t)
	clearTokenEnv(t)
	t.Setenv("GITHUB_TOKEN", "from-env")

	got, err := resolveToken(t.Context(), ProviderGitHub, Options{TokenCommand: "exit 1"})
	if err == nil {
		t.Fatalf("resolveToken(failing command) = %q, want error", got)
	}
	if !strings.Contains(err.Error(), string(ProviderGitHub)) {
		t.Errorf("error = %v, want it to name the provider", err)
	}
}

// TestNewGitProviderUsesTokenCommand proves the command output reaches
// construction: with no token and no env, only the command can satisfy it.
//
//nolint:paralleltest // clearTokenEnv calls t.Setenv, which is incompatible with t.Parallel.
func TestNewGitProviderUsesTokenCommand(t *testing.T) {
	if runtimeIsWindows() {
		t.Skip("shell fixtures assume a POSIX shell")
	}
	resetTokenCache(t)
	clearTokenEnv(t)

	p, err := NewGitProvider(t.Context(), "github", Options{TokenCommand: "echo tok"})
	if err != nil {
		t.Fatalf("NewGitProvider: %v", err)
	}
	if got := typeName(p); got != "*providers.gitHubProvider" {
		t.Errorf("NewGitProvider type = %s, want *providers.gitHubProvider", got)
	}

	if _, err := NewGitProvider(t.Context(), "github", Options{TokenCommand: "exit 1"}); err == nil {
		t.Error("NewGitProvider(failing token command) = nil error, want error")
	}
}

func TestFirstNonEmptyLine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		out  string
		want string
	}{
		{"tok\n", "tok"},
		{"\r\ntok\r\n", "tok"},
		{"\n\n  tok  \nmore\n", "tok"},
		{"", ""},
		{"\n \t\n", ""},
	}
	for _, tt := range tests {
		if got := firstNonEmptyLine(tt.out); got != tt.want {
			t.Errorf("firstNonEmptyLine(%q) = %q, want %q", tt.out, got, tt.want)
		}
	}
}
