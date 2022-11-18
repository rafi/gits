package domain

import (
	"fmt"
	"path/filepath"
	"strings"

	aur "github.com/logrusorgru/aurora"
	homedir "github.com/mitchellh/go-homedir"
	log "github.com/sirupsen/logrus"
)

// Project represents a single project.
type Project struct {
	Name    string       `string:"name"`
	Path    string       `string:"path"`
	AbsPath string       `string:"abspath"`
	Desc    string       `string:"desc"`
	Repos   []Repository `mapstructure:"repos"`
}

// GetTitle returns a colored project title.
func (project Project) GetTitle() string {
	desc := project.Desc
	if desc != "" {
		desc = " (" + desc + ")"
	}

	return fmt.Sprintf("%v %v%v\n", aur.Blue("::"), project.Name, desc)
}

// GetMaxLen returns length of the widest repo directory in a project.
func (project Project) GetMaxLen() int {
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

// GetRepoAbsPath returns an absolute path of a repo directory.
func (project Project) GetRepoAbsPath(path string) (string, error) {
	var err error
	path, err = homedir.Expand(path)

	if len(project.AbsPath) > 0 && string(path[0]) != "/" {
		path = filepath.Join(project.AbsPath, path)
	}

	return path, err
}
