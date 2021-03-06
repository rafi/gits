package common

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/karrick/godirwalk"
	log "github.com/sirupsen/logrus"
)

// GitRun executes git command-line with provided arguments
func GitRun(path string, args []string, crash bool) []byte {
	var (
		cmdOut []byte
		err    error
	)
	cmdName := "git"
	args = append([]string{"-C", path}, args...)

	cmd := exec.Command(cmdName, args...)
	if cmdOut, err = cmd.CombinedOutput(); err != nil {
		if crash {
			log.Error(fmt.Sprintf("Failed to run %v\n", args))
			log.Fatal(fmt.Sprintf("%s %s", cmdOut, err))
		} else {
			return nil
		}
	}
	return cmdOut
}

func GitIsRepo(path string) bool {
	cmdName := "git"
	args := []string{"-C", path, "rev-parse", "--is-inside-work-tree"}
	result := exec.Command(cmdName, args...)
	if err := result.Run(); err != nil {
		return false
	}
	return true
}

// GitDiscoverRepos recursively search for git repositories
func GitDiscoverRepos(path string) ([]RepoInfo, error) {
	var repos []RepoInfo

	err := godirwalk.Walk(path, &godirwalk.Options{
		Callback: func(osPathname string, de *godirwalk.Dirent) error {
			if de.IsDir() {
				_, err := os.Stat(filepath.Join(osPathname, ".git"))
				if !os.IsNotExist(err) {
					repo := RepoInfo{"dir": osPathname}
					repos = append(repos, repo)
					// Stop searching in current directory
					return filepath.SkipDir
				}
			}
			return nil
		},
		ErrorCallback: func(osPathname string, err error) godirwalk.ErrorAction {
			_, err = fmt.Fprintf(os.Stderr, "ERROR: %s\n", err)
			if err != nil {
				log.Fatal(err)
			}
			return godirwalk.SkipNode
		},
		Unsorted: true,
	})

	return repos, err
}
