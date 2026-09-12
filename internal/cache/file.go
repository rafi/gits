package cache

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/mitchellh/go-homedir"
	log "github.com/sirupsen/logrus"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/fsutil"
	"github.com/rafi/gits/internal/version"
)

const cacheTimeFormat = time.RFC3339

// File is the file-backed cache client. Its ttl comes from the cacheTTL
// setting: an explicit "0s" disables caching entirely (every Get is a miss).
type File struct {
	ttl time.Duration
}

// payload is the on-disk cache format.
type payload struct {
	Version   string         `json:"version"`
	Timestamp string         `json:"timestamp"`
	Checksum  string         `json:"checksum"`
	Project   domain.Project `json:"project"`
}

// NewFileCache returns a file-backed cache client with the given ttl.
func NewFileCache(ttl time.Duration) Cacher {
	return &File{ttl: ttl}
}

func cacheFilePath(key string) (string, error) {
	var err error
	path := os.Getenv("XDG_CACHE_HOME")
	if path == "" {
		path = "~/.cache"
	}
	path = filepath.Join(path, "gits", key+".json")
	path, err = homedir.Expand(path)
	if err != nil {
		return "", fmt.Errorf("failed to expand cache path: %w", err)
	}
	return path, nil
}

func (cf *File) Get(key string, project *domain.Project) (bool, error) {
	path, err := cacheFilePath(key)
	if err != nil {
		return false, fmt.Errorf("failed to get cache file path: %w", err)
	}
	fp, err := os.Open(path)
	if os.IsNotExist(err) {
		log.Debug("cache file not found")
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to open cache file: %w", err)
	}
	defer fp.Close()

	// Read the file content
	content, err := io.ReadAll(fp)
	if err != nil {
		return false, fmt.Errorf("failed to read cache file: %w", err)
	}

	// Parse the JSON content into a local value so one read can never leak
	// state into the next. A corrupt cache file (interrupted write, disk
	// full) is a miss to be refreshed, never a hard error.
	var p payload
	if err := json.Unmarshal(content, &p); err != nil {
		log.Warnf("ignoring corrupt cache file %s: %v", path, err)
		return false, nil
	}

	// Bust cache if version or checksum mismatch
	if p.Version != version.GetMajorMinor() {
		log.Debugf(
			"version mismatch %s != %s. busting cache.",
			p.Version,
			version.GetMajorMinor(),
		)
		return false, nil
	}
	if p.Checksum != project.Hash {
		log.Debug("checksum mismatch. busting cache.")
		return false, nil
	}

	// Bust cache if expired; a zero ttl expires everything immediately.
	cutoff := time.Now().Add(-cf.ttl)
	cachedAt, err := time.Parse(cacheTimeFormat, p.Timestamp)
	if err != nil {
		log.Warnf("failed to parse cache timestamp: %v", err)
		return false, nil
	}
	if cachedAt.Before(cutoff) {
		log.Debug("cache expired")
		return false, nil
	}
	*project = p.Project
	return true, nil
}

func (cf *File) Save(key string, project domain.Project) error {
	path, err := cacheFilePath(key)
	if err != nil {
		return fmt.Errorf("failed to get cache file path: %w", err)
	}

	basePath := filepath.Dir(path)
	if err := os.MkdirAll(basePath, 0o755); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	cacheRaw, err := json.Marshal(payload{
		Version:   version.GetMajorMinor(),
		Timestamp: time.Now().Format(cacheTimeFormat),
		Checksum:  project.Hash,
		Project:   project,
	})
	if err != nil {
		return fmt.Errorf("failed to encode cache file: %w", err)
	}

	// Write via temp file + rename so an interrupted write can never leave
	// a truncated cache file behind.
	if err := fsutil.WriteFileAtomic(path, cacheRaw, 0o644); err != nil {
		return fmt.Errorf("failed to write cache file: %w", err)
	}
	return nil
}

func (cf *File) Flush(project domain.Project) error {
	if project.Source == nil {
		return fmt.Errorf("project %q has no source", project.Name)
	}
	if err := project.Source.Validate(); err != nil {
		return err
	}
	cachePath, err := cacheFilePath(project.Source.UniqueKey())
	if err != nil {
		return err
	}
	// An absent cache file means there is nothing to flush.
	if err := os.Remove(cachePath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("failed to remove cache file: %w", err)
	}
	return nil
}
