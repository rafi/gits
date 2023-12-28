package project

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mitchellh/go-homedir"
	log "github.com/sirupsen/logrus"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/version"
)

const (
	cacheTimeFormat = time.RFC3339
	cacheTTL        = 24 * time.Hour
)

type cacheStub struct {
	Version   string         `json:"version"`
	Timestamp string         `json:"timestamp"`
	MD5       string         `json:"md5"`
	Project   domain.Project `json:"project"`
}

func newProjectCache(project domain.Project, checksum string) cacheStub {
	return cacheStub{
		Version:   version.GetVersion(),
		Timestamp: time.Now().Format(cacheTimeFormat),
		MD5:       checksum,
		Project:   project,
	}
}

func CleanCache(project domain.Project) error {
	id, err := project.Source.GetFilterID()
	if id == "" {
		return fmt.Errorf("project %q error: %w", project.Name, err)
	}
	cacheKey := makeCacheKey(project.Source.Type, id)
	cachePath, err := cacheFilePath(cacheKey)
	if err != nil {
		return err
	}
	if _, err := os.Stat(cachePath); err == nil {
		err := os.Remove(cachePath)
		if err != nil {
			return fmt.Errorf("failed to remove cache file: %w", err)
		}
	}
	return nil
}

func makeCacheKey(sourceType, id string) string {
	return fmt.Sprintf("%s-%s", sourceType, strings.ReplaceAll(id, "/", "%"))
}

func md5sum(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := md5.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
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

func getCache(key, checksum string, project *domain.Project) (bool, error) {
	path, err := cacheFilePath(key)
	if err != nil {
		return false, fmt.Errorf("failed to get cache file path: %w", err)
	}
	fp, err := os.Open(path)
	if os.IsNotExist(err) {
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
	var cached cacheStub
	err = json.Unmarshal(content, &cached)
	if err != nil {
		return false, fmt.Errorf("failed to parse cache file: %w", err)
	}

	// Bust cache if version or checksum mismatch
	if cached.Version != version.GetVersion() {
		return false, nil
	}
	if cached.MD5 != checksum {
		return false, nil
	}

	// Bust cache if older than 24 hours
	cutoff := time.Now().Add(-cacheTTL)
	cachedAt, err := time.Parse(cacheTimeFormat, cached.Timestamp)
	if err != nil {
		log.Warnf("failed to parse cache timestamp: %v", err)
		return false, nil
	}
	if cachedAt.Before(cutoff) {
		return false, nil
	}
	*project = cached.Project
	return true, nil
}

func saveCache(key, checksum string, project domain.Project) error {
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
	cache := newProjectCache(project, checksum)
	cacheRaw, err = json.Marshal(cache)
	if err != nil {
		return fmt.Errorf("failed to parse cache file: %w", err)
	}

	fp, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create cache file: %w", err)
	}
	defer fp.Close()

	_, err = io.WriteString(fp, string(cacheRaw))
	if err != nil {
		return fmt.Errorf("failed to read cache file: %w", err)
	}
	return nil
}
