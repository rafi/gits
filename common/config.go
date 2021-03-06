package common

import (
	"fmt"
	"path/filepath"
	"strings"

	aur "github.com/logrusorgru/aurora"
	homedir "github.com/mitchellh/go-homedir"
	log "github.com/sirupsen/logrus"
)

// RepoInfo represents a single repository
type RepoInfo map[string]string

// ProjectInfo represents a single project
type ProjectInfo struct {
	Name    string     `string:"name"`
	Path    string     `string:"path"`
	AbsPath string     `string:"abspath"`
	Desc    string     `string:"desc"`
	Repos   []RepoInfo `mapstructure:"repos"`
}

// Config is the root of configuration
type Config struct {
	Projects map[string]ProjectInfo `mapstructure:"projects"`
	Verbose  bool
}

// GetProject correctly returns a proper project object
func (config Config) GetProject(name string) (ProjectInfo, error) {
	var err error
	var project ProjectInfo
	if name == "." || name[0:1] == "/" || name[0:2] == "./" {
		project.Path, err = filepath.Abs(name)
		if err == nil {
			project.Name = filepath.Base(project.Path)
			project.Repos, err = GitDiscoverRepos(project.Path)
		}
	} else {
		project = config.Projects[name]
		project.AbsPath, err = homedir.Expand(project.Path)
		project.Name = name
	}

	return project, err
}

// GetTitle returns a colored project title
func (project ProjectInfo) GetTitle() string {
	desc := project.Desc
	if desc != "" {
		desc = " (" + desc + ")"
	}
	return fmt.Sprintf("%v %v%v\n", aur.Blue("::"), project.Name, desc)
}

// GetMaxLen returns length of the widest repo directory in a project
func (project ProjectInfo) GetMaxLen() int {
	// Find home directory
	home, err := homedir.Dir()
	if err != nil {
		log.Fatal("Unable to find home directory, ", err)
	}

	maxLen := 0
	for _, repoCfg := range project.Repos {
		if i := len(strings.Replace(repoCfg["dir"], home, "~", 1)); i > maxLen {
			maxLen = i
		}
	}
	return maxLen
}

// GetRepoAbsPath returns an absolute path of a repo directory
func (project ProjectInfo) GetRepoAbsPath(path string) (string, error) {
	var err error
	path, err = homedir.Expand(path)
	if len(project.AbsPath) > 0 && string(path[0]) != "/" {
		path = filepath.Join(project.AbsPath, path)
	}
	return path, err
}
