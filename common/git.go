package common

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"os/exec"
)

func GitRun(path string, args []string, crash bool) []byte {
	var (
		cmdOut []byte
		err    error
	)
	cmdName := "git"
	args = append([]string{"-C", path}, args...)
	if cmdOut, err = exec.Command(cmdName, args...).Output(); err != nil {
		if crash == true {
			log.Error(fmt.Sprintf("Failed to run %v\n", args))
			log.Fatal(err)
		} else {
			return nil
		}
	}
	return cmdOut
}
