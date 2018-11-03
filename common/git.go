package common

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"os/exec"
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
		if crash == true {
			log.Error(fmt.Sprintf("Failed to run %v\n", args))
			log.Fatal(err)
		} else {
			return nil
		}
	}
	return cmdOut
}
