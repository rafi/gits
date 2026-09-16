package edit

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/rafi/gits/internal/config"
)

// EnsurePath returns the config file a command should write to, creating an
// empty one for a user who has none, and reports whether it had to create it.
// Saying so is left to the caller: this package has no output destination, and
// the sentence belongs to whichever command the user ran.
//
// A user with a config file always gets that file. The empty configPath this
// acts on means the loader searched every default location and found nothing,
// and the new file is created with O_EXCL, so a file that appeared in the
// meantime is used as it is rather than truncated. Nothing here ever writes
// over a config that already exists.
func EnsurePath(configPath string) (path string, created bool, err error) {
	if configPath != "" {
		return configPath, false, nil
	}
	// Re-run the loader's own search: configPath is empty both for a user with
	// no config file and for a test that left the field unset.
	existing, err := config.ExistingPath()
	if err != nil {
		return "", false, err
	}
	if existing != "" {
		return existing, false, nil
	}

	path, err = config.NewFilePath()
	if err != nil {
		return "", false, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", false, fmt.Errorf("unable to create config directory: %w", err)
	}
	// A config file can hold provider tokens, so a file gits creates is the
	// user's own to read; one that already exists keeps the mode they chose.
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
	switch {
	case os.IsExist(err):
		// Another process wrote one between the search and now: append to it.
		return path, false, nil
	case err != nil:
		return "", false, fmt.Errorf("unable to create config file: %w", err)
	}
	if err := file.Close(); err != nil {
		return "", false, fmt.Errorf("unable to create config file: %w", err)
	}
	return path, true, nil
}
