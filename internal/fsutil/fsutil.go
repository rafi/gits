// Package fsutil provides shared filesystem helpers.
package fsutil

import (
	"os"
	"path/filepath"
)

// WriteFileAtomic writes data to path atomically: the temp file is created
// next to the target so the final rename never crosses filesystems, and any
// failure leaves no temp file behind.
func WriteFileAtomic(path string, data []byte, mode os.FileMode) error {
	tmpFile, err := os.CreateTemp(filepath.Dir(path), ".gits-*")
	if err != nil {
		return err
	}
	tmpName := tmpFile.Name()
	defer func() {
		// Best-effort cleanup: the Close that matters is checked below, and
		// a temp file that outlives a failure is not worth a second error.
		_ = tmpFile.Close()
		_ = os.Remove(tmpName)
	}()

	if _, err := tmpFile.Write(data); err != nil {
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// WriteFileAtomicPreserve is like WriteFileAtomic, but preserves the mode of
// an existing file at path, falling back to mode when the file is absent.
func WriteFileAtomicPreserve(path string, data []byte, mode os.FileMode) error {
	if fi, err := os.Stat(path); err == nil {
		mode = fi.Mode().Perm()
	}
	return WriteFileAtomic(path, data, mode)
}
