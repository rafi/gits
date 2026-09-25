package list

import (
	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/app"
)

// RenderSample renders a small fixed project tree through the real tree
// renderer, for `gits theme show`.
func RenderSample(deps app.RuntimeCLI) {
	projects := domain.ProjectListKeyed{
		"acme": {
			Name: "acme", Desc: "sample project", AbsPath: "/src/acme",
			Source: &domain.ProviderSource{Type: "github"},
			Repos:  []domain.Repository{{Name: "api"}, {Name: "web"}},
			SubProjects: []domain.Project{
				{Name: "tools", AbsPath: "/src/acme/tools", Repos: []domain.Repository{{Name: "cli"}}},
			},
		},
	}
	_ = listTree(projects, deps)
}
