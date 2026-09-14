// Package target turns command arguments into the project and repository a
// command runs on. The lookup is pure; when an argument is absent the answer
// comes from a [Selector], so a caller that cannot prompt still resolves.
package target

import (
	"fmt"
	"strings"

	"github.com/rafi/gits/domain"
	coreruntime "github.com/rafi/gits/internal/runtime"
	"github.com/rafi/gits/internal/runtime/projects"
)

// Args is what the command line said, already split. An empty Project or Repo
// means "ask the Selector".
type Args struct {
	// Project is the project name or path.
	Project string
	// Sub is a sub-project, written with a trailing "/" on the command line.
	Sub string
	// Repo is a repository name.
	Repo string
	// RequireRepo demands a repository: without one the command cannot command.
	RequireRepo bool
}

// Parse splits raw command-line arguments into Args. A trailing "/" on the
// second argument means sub-project, not repository.
func Parse(args []string, requireRepo bool) Args {
	a := Args{RequireRepo: requireRepo}
	if len(args) > 0 {
		a.Project = args[0]
	}
	if len(args) > 1 {
		if strings.HasSuffix(args[1], "/") {
			a.Sub = args[1]
		} else {
			a.Repo = args[1]
		}
	}
	return a
}

// Project loads the project named name, descending into the sub-project sub
// when it is set. Naming something that does not exist is a real failure, not
// a downgradeable warning: a script must be able to tell a typo from success.
func Project(name, sub string, rt coreruntime.Runtime) (domain.Project, error) {
	p, err := projects.LoadOne(name, rt)
	if err != nil {
		return p, fmt.Errorf("unable to load project: %w", err)
	}
	if sub == "" {
		return p, nil
	}
	p, found := p.GetSubProject(sub, "")
	if !found {
		return p, fmt.Errorf("project %q not found", sub)
	}
	p.Name = sub
	return p, nil
}

// Repo returns the repository named name within p.
func Repo(p domain.Project, name string) (domain.Repository, error) {
	repo, found := p.GetRepo(name, "")
	if !found {
		return repo, fmt.Errorf("repo %q not found", name)
	}
	return repo, nil
}

// Resolve returns the project and, when asked for, the repository. The
// repository comes back nil only when the command does not require one and no
// second argument named one.
func Resolve(a Args, s Selector, rt coreruntime.Runtime) (
	domain.Project, *domain.Repository, error,
) {
	name := a.Project
	if name == "" {
		var err error
		name, err = s.Project(rt.Ctx)
		if err != nil {
			return domain.Project{}, nil, err
		}
		if name == "" {
			return domain.Project{}, nil, domain.NewWarning("no project selected")
		}
	}

	project, err := Project(name, a.Sub, rt)
	if err != nil {
		return project, nil, err
	}

	if !a.RequireRepo && a.Repo == "" {
		return project, nil, nil
	}

	repoName := a.Repo
	if repoName == "" {
		// The preview re-invokes gits with the root project, which for a path
		// argument is the name the loader derived from it. Only a sub-project
		// has a root distinct from itself.
		root := ""
		if a.Sub != "" {
			root = projects.ProjectName(a.Project)
		}
		repoName, err = s.Repo(rt.Ctx, project, root)
		if err != nil {
			return project, nil, err
		}
		if repoName == "" {
			return project, nil, domain.NewWarning("no repository selected")
		}
	}

	repo, err := Repo(project, repoName)
	if err != nil {
		return project, nil, err
	}
	return project, &repo, nil
}
