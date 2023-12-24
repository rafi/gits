package types

import (
	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/pkg/git"
)

type RuntimeDeps struct {
	Projects domain.ProjectListKeyed
	Settings domain.Settings
	Git      git.Git
	Theme    Theme
	HomeDir  string
	Source   string
}
