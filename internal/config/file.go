package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/charmbracelet/lipgloss"
	"github.com/knadh/koanf/parsers/json"
	"github.com/knadh/koanf/parsers/toml"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
	"github.com/mitchellh/go-homedir"
	"github.com/muesli/termenv"

	"github.com/rafi/gits/domain"
)

// File represents a config file with projects and settings.
type File struct {
	client   *koanf.Koanf
	Projects domain.ProjectListKeyed

	deprecations

	Filename string
	Color    string
	Settings domain.Settings
}

type deprecations struct {
	Projects domain.ProjectListKeyed `koanf:"projects"`
}

// NewConfigFromFile reads in config file and ENV variables if set.
func NewConfigFromFile(filePath string, cfg *File) error {
	if filePath == "" {
		var err error
		filePath, err = cfg.findDefaultProvider()
		if err != nil {
			return fmt.Errorf("unable to find config file: %w", err)
		}
		if filePath == "" {
			return nil
		}
	}

	if err := cfg.loadConfig(filePath); err != nil {
		return fmt.Errorf("unable to load config: %w", err)
	}
	return nil
}

// Convert handles deprecated config formats.
func (f *File) Convert() error {
	if err := f.client.Unmarshal("", &f.deprecations); err != nil {
		return fmt.Errorf("unable to check deprecations: %w", err)
	}
	if len(f.deprecations.Projects) > 0 {
		f.Projects = f.deprecations.Projects
		return errors.New("key 'projects:' is deprecated, remove it")
	}
	return nil
}

// findDefaultProvider reads in config file and ENV variables if set.
func (f File) findDefaultProvider() (string, error) {
	home, err := homedir.Dir()
	if err != nil {
		return "", fmt.Errorf("unable to find home directory: %w", err)
	}

	xdgConfigHome := os.Getenv("XDG_CONFIG_HOME")
	if xdgConfigHome == "" {
		xdgConfigHome = filepath.Join(home, ".config")
	}
	configDirectories := []string{
		filepath.Join(home, ".gits"),
		filepath.Join(xdgConfigHome, "gits", "config"),
	}
	for _, configPath := range configDirectories {
		for _, configExt := range []string{".json", ".yaml", ".toml"} {
			if _, err := os.Stat(configPath + configExt); !os.IsNotExist(err) {
				return configPath + configExt, nil
			}
		}
	}
	return "", nil
}

// loadConfig reads in config file and ENV variables if set.
func (f *File) loadConfig(filePath string) error {
	f.client = koanf.New(".")
	f.Filename = filePath
	provider := file.Provider(filePath)
	fileExt := filepath.Ext(filePath)
	switch fileExt {
	case ".json":
		f.client.Load(provider, json.Parser())
	case ".toml":
		f.client.Load(provider, toml.Parser())
	case ".yaml", ".yml":
		f.client.Load(provider, yaml.Parser())
	default:
		return fmt.Errorf("unsupported config file format: %s", fileExt)
	}

	koanfConf := koanf.UnmarshalConf{Tag: "json"}
	if err := f.client.UnmarshalWithConf("", &f.Projects, koanfConf); err != nil {
		return fmt.Errorf("unable to parse config file: %w", err)
	}

	// Delete the special key saved for built-in CLI settings.
	delete(f.Projects, "settings")

	// Handle deprecated config fields.
	if err := f.Convert(); err != nil {
		fmt.Fprintf(os.Stderr, "%s from %s\n", err, f.Filename)
	}

	// Parse special key 'settings'.
	if err := f.client.UnmarshalWithConf("settings", &f.Settings, koanfConf); err != nil {
		return fmt.Errorf("unable to parse config file: %w", err)
	}

	// Set never/always color toggle.
	switch f.Color {
	case colorOptionNever.String():
		lipgloss.SetColorProfile(termenv.Ascii)
	case colorOptionAlways.String():
		os.Setenv("CLICOLOR_FORCE", "1")
	}
	return nil
}
