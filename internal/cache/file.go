package cache

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/mitchellh/go-homedir"
	log "github.com/sirupsen/logrus"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/version"
)

const (
	cacheTimeFormat = time.RFC3339
	cacheTTL        = 7 * 24 * time.Hour
)

type File struct {
	Version   string         `json:"version"`
	Timestamp string         `json:"timestamp"`
	Checksum  string         `json:"checksum"`
	Project   domain.Project `json:"project"`
}

func newCacheFile() (Cacher, error) {
	cf := &File{}
	return cf, nil
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
	if err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("failed to open cache file: %w", err)
	}
	defer fp.Close()

	// Read the file content
	content, err := io.ReadAll(fp)
	if err != nil {
		return false, fmt.Errorf("failed to read cache file: %w", err)
	}

	// Parse the JSON content
	err = json.Unmarshal(content, cf)
	if err != nil {
		return false, fmt.Errorf("failed to parse cache file: %w", err)
	}

	// Bust cache if version or checksum mismatch
	if cf.Version != version.GetMajorMinor() {
		log.Debugf(
			"version mismatch %s != %s. busting cache.",
			cf.Version,
			version.GetMajorMinor(),
		)
		return false, nil
	}
	if cf.Checksum != project.Hash {
		log.Debug("checksum mismatch. busting cache.")
		return false, nil
	}

	// Bust cache if expired
	cutoff := time.Now().Add(-cacheTTL)
	cachedAt, err := time.Parse(cacheTimeFormat, cf.Timestamp)
	if err != nil {
		log.Warnf("failed to parse cache timestamp: %v", err)
		return false, nil
	}
	if cachedAt.Before(cutoff) {
		log.Debug("cache expired")
		return false, nil
	}
	*project = cf.Project
	return true, nil
}

func (cf *File) Save(key string, project domain.Project) error {
	path, err := cacheFilePath(key)
	if err != nil {
		return fmt.Errorf("failed to get cache file path: %w", err)
	}

	basePath := filepath.Dir(path)
	if _, err := os.Stat(path); err != nil && os.IsNotExist(err) {
		if err := os.MkdirAll(basePath, 0755); err != nil {
			return fmt.Errorf("failed to create cache directory: %w", err)
		}
	}

	var cacheRaw []byte
	cf.Timestamp = time.Now().Format(cacheTimeFormat)
	cf.Project = project
	cf.Version = version.GetMajorMinor()
	cf.Checksum = project.Hash

	cacheRaw, err = json.Marshal(cf)
	if err != nil {
		return fmt.Errorf("failed to parse cache file: %w", err)
	}

	fp, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create cache file: %w", err)
	}
	defer fp.Close()

	_, err = fp.Write(cacheRaw)
	if err != nil {
		return fmt.Errorf("failed to read cache file: %w", err)
	}
	return nil
}

func (cf *File) Flush(project domain.Project) error {
	if err := project.Source.Validate(); err != nil {
		return err
	}
	cachePath, err := cacheFilePath(project.Source.UniqueKey())
	if err != nil {
		return err
	}
	if _, err := os.Stat(cachePath); err == nil {
		err := os.Remove(cachePath)
		if err != nil {
			return fmt.Errorf("failed to remove cache file: %w", err)
		}
	} else {
		return err
	}
	return nil
}
