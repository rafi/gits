package cli

import (
	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/pkg/git"
)

const (
	LeftMargin        = 2
	RightMargin       = 2
	NameColor   uint8 = 12
)

type RuntimeDeps struct {
	Projects domain.ProjectListKeyed
	Settings domain.Settings
	Git      git.Git
	Theme    Theme
	HomeDir  string
	Source   string
}
