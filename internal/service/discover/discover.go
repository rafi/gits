// Package discover groups git repositories on disk by their parent directory
// into candidate Projects. It writes nothing.
package discover

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/mitchellh/go-homedir"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/infra/providers"
	"github.com/rafi/gits/internal/logging"
	coreruntime "github.com/rafi/gits/internal/runtime"
)

// DefaultMin is the default minimum of repositories for a Project.
const DefaultMin = 1

// Group is a directory of git repositories that becomes a Project.
type Group struct {
	// Name is the directory's base name, qualified by its parents when
	// already used in this scan.
	Name string
	// Path is the absolute directory.
	Path string
	// Repos are the repositories directly inside Path, sorted by name.
	Repos []Repo
	// Existing names the configured Project to append Repos to, and
	// ExistingPath is its path, empty when it has none above the directory.
	Existing     string
	ExistingPath string
	// ExistingNames is the top-level key and Sub-project names of Existing.
	ExistingNames []string
	// Taken reports that a Project of this name is already configured.
	Taken bool
}

// Repo is one repository a Group holds.
type Repo struct {
	// Path is the clone's absolute directory.
	Path string
	// Src is the remote URL, empty when the clone has none.
	Src string
}

// Result is everything one scan found.
type Result struct {
	// Groups are the Projects to write, ordered by path.
	Groups []Group
	// Covered counts repositories configured Projects already hold.
	Covered int
	// BelowMin counts repositories in directories holding fewer than Min.
	BelowMin int
}

// Options shape one scan.
type Options struct {
	// Min is the smallest number of repositories a directory must hold.
	// Zero means DefaultMin.
	Min int
	// Existing are the configured Projects; what they cover is skipped.
	Existing domain.ProjectListKeyed
}

// Scan walks root and groups git repositories by their parent directory.
func Scan(root string, rt coreruntime.Runtime, opts Options) (Result, error) {
	absRoot, err := absolute(root)
	if err != nil {
		return Result{}, err
	}
	if info, statErr := os.Stat(absRoot); statErr != nil || !info.IsDir() {
		return Result{}, fmt.Errorf("unable to scan %s: not a directory", root)
	}

	minRepos := opts.Min
	if minRepos <= 0 {
		minRepos = DefaultMin
	}
	covered := coveredPaths(opts.Existing)

	// byParent maps each directory to its repositories. The walk does not
	// descend into repositories.
	byParent := map[string][]string{}
	result := Result{}
	err = providers.WalkRepos(rt.Ctx, rt.Log, absRoot, rt.Git, func(path string) error {
		if covered.holds(path) {
			result.Covered++
			return nil
		}
		parent := filepath.Dir(path)
		byParent[parent] = append(byParent[parent], path)
		return nil
	})
	if err != nil {
		return Result{}, err
	}

	parents := make([]string, 0, len(byParent))
	for parent, repos := range byParent {
		if len(repos) >= minRepos {
			parents = append(parents, parent)
			continue
		}
		result.BelowMin += len(repos)
	}
	sort.Strings(parents)

	result.Groups = make([]Group, 0, len(parents))
	// used holds names already assigned in this scan.
	used := map[string]bool{}
	for _, parent := range parents {
		repos := byParent[parent]
		sort.Strings(repos)
		owner := covered.owner(parent)
		name := owner.name()
		if name == "" {
			name = uniqueName(parent, used)
			used[name] = true
		}
		// Appended to an existing Project, so its name cannot be taken.
		_, taken := opts.Existing[name]
		taken = taken && len(owner.names) == 0
		group := Group{
			Name:          name,
			Path:          parent,
			Existing:      owner.name(),
			ExistingNames: owner.names,
			ExistingPath:  owner.path,
			Taken:         taken,
		}
		if taken {
			// Never written; skip reading remotes.
			group.Repos = withoutRemotes(repos)
		} else {
			group.Repos = withRemotes(rt, repos)
		}
		result.Groups = append(result.Groups, group)
	}
	return result, nil
}

// uniqueName returns path's base name, prefixed with parent directories
// until it is not in used, e.g. `work-archive`.
func uniqueName(path string, used map[string]bool) string {
	name := filepath.Base(path)
	parent := filepath.Dir(path)
	for used[name] {
		base := filepath.Base(parent)
		if next := filepath.Dir(parent); next != parent && base != string(filepath.Separator) {
			name = base + "-" + name
			parent = next
			continue
		}
		// Path exhausted; append a counter.
		for n := 2; ; n++ {
			numbered := fmt.Sprintf("%s-%d", name, n)
			if !used[numbered] {
				return numbered
			}
		}
	}
	return name
}

// withRemotes lists clones with their remote URLs. Clones without a remote
// are kept with an empty Src.
func withRemotes(rt coreruntime.Runtime, paths []string) []Repo {
	out := make([]Repo, 0, len(paths))
	for _, path := range paths {
		src, err := rt.Git.Remote(rt.Ctx, path)
		if err != nil {
			logging.Or(rt.Log).DebugContext(rt.Ctx, "no remote for discovered repository",
				"path", path, "err", err)
			src = ""
		}
		out = append(out, Repo{Path: path, Src: src})
	}
	return out
}

// withoutRemotes lists clones without asking git for their remotes.
func withoutRemotes(paths []string) []Repo {
	out := make([]Repo, 0, len(paths))
	for _, path := range paths {
		out = append(out, Repo{Path: path})
	}
	return out
}

// Total returns how many repositories the scan found.
func (r Result) Total() int {
	total := r.Covered + r.BelowMin
	for _, g := range r.Groups {
		total += len(g.Repos)
	}
	return total
}

// Writable returns the groups whose name is not taken.
func (r Result) Writable() []Group {
	out := make([]Group, 0, len(r.Groups))
	for _, g := range r.Groups {
		if !g.Taken {
			out = append(out, g)
		}
	}
	return out
}

// coverage is what configured Projects hold. A Project with a source covers
// its whole path; one that lists repositories covers only those.
type coverage struct {
	// trees are paths covered recursively.
	trees []string
	// repos are directories Projects list.
	repos []string
	// listed are paths of Projects that list their repositories.
	listed []owned
	// holding are parent directories of listed repositories, not recursive.
	holding []owned
}

// owned is a configured Project's path and name.
type owned struct {
	path string
	// names is the top-level key and Sub-project names of the Project.
	names []string
}

// name returns the owner's display name, `parent/child` for a Sub-project.
func (o owned) name() string {
	return strings.Join(o.names, "/")
}

// coveredPaths returns the coverage of unloaded, as-read Projects.
func coveredPaths(projects domain.ProjectListKeyed) coverage {
	covered := coverage{}
	for key, project := range projects {
		covered.collect(project, []string{key}, "")
	}
	return covered
}

// collect adds a Project and its Sub-projects to the coverage. parentPath
// is the parent's absolute path.
func (c *coverage) collect(project domain.Project, names []string, parentPath string) {
	path, explicit := "", false
	switch {
	case project.Path != "":
		if abs, err := absolute(project.Path); err == nil {
			path, explicit = abs, true
		}
	case parentPath != "" && len(names) > 1:
		path = filepath.Join(parentPath, project.Name)
	}
	switch {
	case explicit && discoversRepos(project):
		// Its source owns the whole path.
		c.trees = append(c.trees, path)
	case path != "" && len(project.Repos) > 0:
		c.listed = append(c.listed, owned{path: path, names: names})
	}
	resolver := domain.Project{AbsPath: path}
	for _, repo := range project.Repos {
		abs := repo.AbsPath
		if abs == "" {
			var err error
			if abs, err = resolver.GetRepoAbsPath(repo); err != nil {
				continue
			}
		}
		abs = filepath.Clean(abs)
		c.repos = append(c.repos, abs)
		c.holding = append(c.holding, owned{path: filepath.Dir(abs), names: names})
	}
	for _, sub := range project.SubProjects {
		c.collect(sub, append(slices.Clone(names), sub.Name), path)
	}
}

// holds reports whether a Project already covers the repository at path.
func (c *coverage) holds(path string) bool {
	return under(path, c.trees) ||
		slices.Contains(c.trees, path) ||
		slices.Contains(c.repos, path)
}

// owner returns the deepest listed Project holding dir, with an empty path
// when it only lists repositories there.
func (c *coverage) owner(dir string) owned {
	best := owned{}
	for _, candidate := range c.listed {
		if candidate.path != dir && !under(dir, []string{candidate.path}) {
			continue
		}
		if len(candidate.path) >= len(best.path) {
			best = candidate
		}
	}
	if len(best.names) > 0 {
		return best
	}
	// Fall back to Projects listing repositories in dir.
	for _, candidate := range c.holding {
		if candidate.path == dir {
			return owned{names: candidate.names}
		}
	}
	return owned{}
}

// discoversRepos reports whether a Project gets its repositories from a
// source, including a path with no repositories listed.
func discoversRepos(project domain.Project) bool {
	if project.Source != nil && project.Source.Type != "" {
		return true
	}
	return len(project.Repos) == 0
}

// under reports whether path is strictly inside any of roots.
func under(path string, roots []string) bool {
	for _, root := range roots {
		if strings.HasPrefix(path, root+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// absolute expands ~ and makes path absolute.
func absolute(path string) (string, error) {
	expanded, err := homedir.Expand(path)
	if err != nil {
		return "", err
	}
	abs, err := filepath.Abs(expanded)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}
