// Package orphan implements `gits orphan`, which finds repositories on disk
// under a Project Path that the Project's config does not declare.
package orphan

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"charm.land/lipgloss/v2"
	"github.com/karrick/godirwalk"
	log "github.com/sirupsen/logrus"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cli"
	"github.com/rafi/gits/internal/git"
	"github.com/rafi/gits/internal/providers"
	"github.com/rafi/gits/internal/types"
)

// ExecOrphan discovers orphaned repositories, ones that are not known to the
// project provider. With a repo argument, the scan is scoped to that
// repository's work tree and reports git repositories embedded inside it.
//
// Args: (optional)
//   - project name
//   - repo or sub-project name
func ExecOrphan(args []string, deps types.RuntimeCLI) error {
	project, repo, err := cli.ParseArgs(args, true, deps)
	if err != nil {
		return err
	}

	var repos []domain.Repository
	if repo != nil {
		if repo.State != domain.RepoStateOK {
			return cli.AbortOnRepoState(deps.Err, *repo, deps.Theme.Error)
		}
		repos, err = findNestedRepos(deps.Ctx, repo.AbsPath, deps.Git)
	} else {
		repos, err = findOrphanedRepos(deps.Ctx, project, deps.Git)
	}
	if err != nil {
		return err
	}

	errorStyle := deps.Theme.Error.
		MarginLeft(cli.LeftMargin)

	lipgloss.Fprintln(deps.Out, cli.ProjectTitleWithBullet(project, deps.Theme))
	for _, repo := range repos {
		repoDir := cli.Path(repo.Dir, deps.HomeDir)
		lipgloss.Fprintf(deps.Out, "%s - %s\n", errorStyle.Render(repoDir), repo.Src)
	}

	return nil
}

// findNestedRepos scans a repository's work tree for embedded git
// repositories — directories with their own .git — which no provider knows
// about. Detection is by .git presence, not rev-parse: inside a work tree
// every subdirectory reports --is-inside-work-tree.
func findNestedRepos(ctx context.Context, root string, gitClient git.Client) ([]domain.Repository, error) {
	orphanRepos := []domain.Repository{}
	walkErr := godirwalk.Walk(root, &godirwalk.Options{
		Unsorted:            false,
		FollowSymbolicLinks: false,
		Callback: func(path string, de *godirwalk.Dirent) error {
			if !de.IsDir() || path == root {
				return nil
			}
			if de.Name() == ".git" {
				return filepath.SkipDir
			}
			// A stat error simply means no .git here: keep descending.
			if _, err := os.Stat(filepath.Join(path, ".git")); err == nil {
				repo, err := providers.NewFilesystemRepo(ctx, path, "", gitClient)
				if err != nil {
					return err
				}
				orphanRepos = append(orphanRepos, repo)
				return filepath.SkipDir
			}
			return nil
		},
		ErrorCallback: func(path string, err error) godirwalk.ErrorAction {
			log.Warnf("skipping %s: %s", path, err)
			return godirwalk.SkipNode
		},
	})
	if walkErr != nil {
		return nil, walkErr
	}
	return orphanRepos, nil
}

// makeRepoMap recursively creates a map of known repository paths.
func makeRepoMap(project domain.Project, repoMap map[string]bool) {
	for _, repo := range project.Repos {
		repoMap[repo.AbsPath] = true
	}
	for _, subProject := range project.SubProjects {
		makeRepoMap(subProject, repoMap)
	}
}

// findOrphanedRepos scans the project's directory for repositories that are not
// known to the project provider.
func findOrphanedRepos(ctx context.Context, project domain.Project, gitClient git.Client) ([]domain.Repository, error) {
	orphanRepos := []domain.Repository{}
	knownRepos := make(map[string]bool)
	makeRepoMap(project, knownRepos)

	if project.AbsPath == "" {
		return nil, fmt.Errorf(
			"project %q has no path, so every repository has an absolute path, aborting",
			project.Name,
		)
	}

	walkErr := providers.WalkRepos(ctx, project.AbsPath, gitClient, func(path string) error {
		// Add unknown repository to the list.
		if _, known := knownRepos[path]; !known {
			repo, err := providers.NewFilesystemRepo(ctx, path, "", gitClient)
			if err != nil {
				return err
			}
			orphanRepos = append(orphanRepos, repo)
		}
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}
	return orphanRepos, nil
}
