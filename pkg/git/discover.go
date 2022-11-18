package git

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/karrick/godirwalk"
)

func (g Git) IsRepo(path string) bool {
	cmdName := "git"
	args := []string{"-C", path, "rev-parse", "--is-inside-work-tree"}
	result := exec.Command(cmdName, args...)
	if err := result.Run(); err != nil {
		return false
	}
	return true
}

// DiscoverRepos recursively search for git repositories.
func (g Git) DiscoverRepos(path string) ([]string, error) {
	var repos []string

	err := godirwalk.Walk(path, &godirwalk.Options{
		Unsorted:            true,
		FollowSymbolicLinks: true,
		Callback: func(osPathname string, de *godirwalk.Dirent) error {
			if de.IsDir() {
				_, err := os.Stat(filepath.Join(osPathname, ".git"))
				if !os.IsNotExist(err) {
					repos = append(repos, osPathname)
					// Stop searching in current directory
					return filepath.SkipDir
				}
			}

			return nil
		},
		ErrorCallback: func(osPathname string, err error) godirwalk.ErrorAction {
			_, err = fmt.Fprintf(os.Stderr, "ERROR: %s\n", err)
			if err != nil {
				fmt.Println(err)
				return godirwalk.Halt
			}

			return godirwalk.SkipNode
		},
	})

	return repos, err
}
