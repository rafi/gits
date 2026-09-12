package providers

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/rafi/gits/internal/logging"
)

// tokenCommandTimeout bounds a settings token command. It is generous on
// purpose: commands like `pass` may wait on a gpg-agent pinentry prompt.
const tokenCommandTimeout = 2 * time.Minute

// tokenCache memoizes token command output for the process lifetime, so N
// projects sharing a provider don't trigger N password prompts.
var (
	tokenCacheMu sync.Mutex
	tokenCache   = map[string]string{}
)

// resolveToken returns the token for a provider, in order of precedence: the
// configured token, the output of the configured token command, and finally
// the provider's environment variables.
func resolveToken(ctx context.Context, provider Provider, opts Options) (string, error) {
	if opts.Token != "" {
		return opts.Token, nil
	}
	if cmd := strings.TrimSpace(opts.TokenCommand); cmd != "" {
		token, err := runTokenCommand(ctx, opts.Log, cmd)
		if err != nil {
			return "", fmt.Errorf("%s token command failed: %w", provider, err)
		}
		return token, nil
	}
	return getFirstEnvValue(tokenEnvVarNames[provider]), nil
}

// runTokenCommand executes command with the system shell and returns the
// first non-empty line it prints, memoized per command string. stdin is not
// connected: interactive helpers are expected to prompt via their own tty
// (e.g. gpg-agent's pinentry), not to read from ours.
func runTokenCommand(ctx context.Context, logger *slog.Logger, command string) (string, error) {
	logger = logging.Or(logger)
	tokenCacheMu.Lock()
	defer tokenCacheMu.Unlock()
	if token, ok := tokenCache[command]; ok {
		logger.DebugContext(ctx, "using cached token", "command", command)
		return token, nil
	}

	ctx, cancel := context.WithTimeout(ctx, tokenCommandTimeout)
	defer cancel()

	logger.DebugContext(ctx, "running token command", "command", command)
	shell, args := shellCommand(command)
	cmd := exec.CommandContext(ctx, shell, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return "", fmt.Errorf("%w: %s", err, msg)
		}
		return "", err
	}

	token := firstNonEmptyLine(string(out))
	if token == "" {
		return "", fmt.Errorf("command %q printed no token", command)
	}
	tokenCache[command] = token
	return token, nil
}

// shellCommand returns the shell invocation running command as written, so
// pipes and quoting behave as they would in a terminal.
func shellCommand(command string) (string, []string) {
	if runtime.GOOS == "windows" {
		return "cmd", []string{"/C", command}
	}
	return "/bin/sh", []string{"-c", command}
}

// firstNonEmptyLine returns the first line of out that holds anything but
// whitespace, trimmed. Token helpers often print a trailing newline, and
// some (e.g. `pass` on a multi-line entry) print extra lines after it.
func firstNonEmptyLine(out string) string {
	for line := range strings.SplitSeq(out, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
