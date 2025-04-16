package orphan

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/karrick/godirwalk"
	log "github.com/sirupsen/logrus"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/cli"
	"github.com/rafi/gits/internal/types"
	"github.com/rafi/gits/pkg/git"
	"github.com/rafi/gits/pkg/providers"
)

// ExecOrphan discovers orphaned repositories, ones that are not known to the
// project provider.
//
// Args: (optional)
//   - project name
//   - sub-project name
func ExecOrphan(args []string, deps types.RuntimeCLI) error {
	project, _, err := cli.ParseArgs(args, true, deps)
	if err != nil {
		return err
	}

	repos, err := findOrphanedRepos(project, deps.Git)
	if err != nil {
		return err
	}

	errorStyle := deps.Theme.Error.
		MarginLeft(cli.LeftMargin)

	fmt.Println(cli.ProjectTitleWithBullet(project, deps.Theme))
	for _, repo := range repos {
		repoDir := cli.Path(repo.Dir, deps.HomeDir)
		fmt.Printf("%s - %s\n", errorStyle.Render(repoDir), repo.Src)
	}

	return nil
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
func findOrphanedRepos(project domain.Project, gitClient git.Git) ([]domain.Repository, error) {
	orphanRepos := []domain.Repository{}
	knownRepos := make(map[string]bool)
	makeRepoMap(project, knownRepos)

	if project.AbsPath == "" {
		return nil, fmt.Errorf(
			"project %q has no path, so every repository has an absolute path, aborting",
			project.Name,
		)
	}

	walkErr := godirwalk.Walk(project.AbsPath, &godirwalk.Options{
		Unsorted:            false,
		FollowSymbolicLinks: false,
		Callback: func(path string, de *godirwalk.Dirent) error {
			if !de.IsDir() || !gitClient.IsRepo(path) {
				return nil
			}
			// Add unknown repository to the list.
			if _, known := knownRepos[path]; !known {
				repo, err := providers.NewFilesystemRepo(path, "", gitClient)
				if err != nil {
					return err
				}
				orphanRepos = append(orphanRepos, repo)
			}
			return filepath.SkipDir
		},
		ErrorCallback: func(path string, err error) godirwalk.ErrorAction {
			_, err = fmt.Fprintf(os.Stderr, "ERROR during directory %s scan: %s\n", path, err)
			if err != nil {
				log.Errorf("findOrphanedRepos: %s", err)
				return godirwalk.Halt
			}
			return godirwalk.SkipNode
		},
	})
	if walkErr != nil {
		return nil, walkErr
	}
	return orphanRepos, nil
}
