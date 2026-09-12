package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/knadh/koanf/parsers/json"
	"github.com/knadh/koanf/parsers/toml"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
	"github.com/mitchellh/go-homedir"

	"github.com/rafi/gits/domain"
)

// minWorkerCount is the smallest bulk worker pool a machine gets, however
// few CPUs it reports.
const minWorkerCount = 2

// File represents a config file with projects and settings.
type File struct {
	deprecations

	client   *koanf.Koanf
	Projects domain.ProjectListKeyed

	Filename string
	Color    string
	Settings domain.Settings

	// Warnings are non-fatal notices gathered while loading (e.g. a deprecated
	// key). They are surfaced as prose on Diagnostic Output by the caller that
	// has a theme and a writer, rather than printed from here — this runs in
	// cobra's initializer, before a runtime exists. See ADR-0004.
	Warnings []string
}

type deprecations struct {
	Projects domain.ProjectListKeyed `koanf:"projects"`
}

// NewConfigFromFile reads in config file and ENV variables if set.
func NewConfigFromFile(filePath string, cfg *File) error {
	// Runtime defaults and the color toggle must apply even when no config
	// file exists or loading fails part-way.
	defer cfg.applyDefaults()

	if filePath == "" {
		var err error
		filePath, err = cfg.findDefaultPath()
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

// applyDefaults fills runtime defaults and applies the never/always color
// toggle. lipgloss v2 downsamples at the output writer, so force the profile
// on the global stdout writer and mirror the intent into the environment so
// per-writer outputs (e.g. the progress reporter's destination) and child
// processes (git, fzf) honor it too.
func (f *File) applyDefaults() {
	if f.Settings.WorkerCount == 0 {
		f.Settings.WorkerCount = max(runtime.NumCPU(), minWorkerCount)
	}
	f.Settings.Icons.ApplyDefaults()

	switch f.Color {
	case ColorOptionNever.String():
		_ = os.Setenv("NO_COLOR", "1")
		lipgloss.Writer.Profile = colorprofile.NoTTY
	case ColorOptionAlways.String():
		_ = os.Setenv("CLICOLOR_FORCE", "1")
		lipgloss.Writer.Profile = colorprofile.TrueColor
	}
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

// validateProjects rejects a project (or sub-project) that sets both a
// Provider Source and an explicit repository list. A source discovers the
// repository list, so a `repos:` beside it has no unambiguous meaning: it was
// silently discarded for remote sources and appended by accident for
// filesystem. The error names the project and both keys.
func validateProjects(projects domain.ProjectListKeyed) error {
	for name, proj := range projects {
		proj.Name = name
		if err := validateProject(proj); err != nil {
			return err
		}
	}
	return nil
}

// validateProject checks one project and its sub-projects for the both-set
// combination.
func validateProject(project domain.Project) error {
	hasSource := project.Source != nil && project.Source.Type != ""
	if hasSource && len(project.Repos) > 0 {
		return fmt.Errorf(
			"project %q sets both `source:` and `repos:`; use one or the other",
			project.Name,
		)
	}
	for _, sub := range project.SubProjects {
		if err := validateProject(sub); err != nil {
			return err
		}
	}
	return nil
}

// findDefaultPath reads in config file and ENV variables if set.
func (f *File) findDefaultPath() (string, error) {
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
		for _, configExt := range []string{".json", ".yaml", ".yml", ".toml"} {
			//nolint:gosec // the path is built from this user's own HOME and
			// XDG_CONFIG_HOME; there is no untrusted input to traverse with.
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
	var err error
	switch fileExt {
	case ".json":
		err = f.client.Load(provider, json.Parser())
	case ".toml":
		err = f.client.Load(provider, toml.Parser())
	case ".yaml", ".yml":
		err = f.client.Load(provider, yaml.Parser())
	default:
		return fmt.Errorf("unsupported config file format: %s", fileExt)
	}
	if err != nil {
		return err
	}

	koanfConf := koanf.UnmarshalConf{Tag: "json"}
	if err := f.client.UnmarshalWithConf("", &f.Projects, koanfConf); err != nil {
		return fmt.Errorf("unable to parse config file: %w", err)
	}

	// Delete the special key saved for built-in CLI settings.
	delete(f.Projects, "settings")

	// A project points at a Provider Source or lists its repositories, not
	// both. This is the one place the raw config is seen before the loader
	// restores a cached repository list onto a source-backed project, so it is
	// the only place the two can be told apart.
	if err := validateProjects(f.Projects); err != nil {
		return err
	}

	// Handle deprecated config fields.
	if err := f.Convert(); err != nil {
		// A deprecation is a warning, not a failure: record it for the caller
		// to render on Diagnostic Output rather than printing to os.Stderr
		// here — see the Warnings field and ADR-0004.
		f.Warnings = append(f.Warnings, fmt.Sprintf("%s from %s", err, f.Filename))
	}

	// Parse special key 'settings'.
	if err := f.client.UnmarshalWithConf("settings", &f.Settings, koanfConf); err != nil {
		return fmt.Errorf("unable to parse config file: %w", err)
	}

	return nil
}
