//go:build !unix

package ecp

import (
	"os"
	"os/exec"
	"time"
)

func configureProcessCancellation(command *exec.Cmd) {
	command.Cancel = func() error {
		if command.Process == nil {
			return os.ErrProcessDone
		}
		return command.Process.Kill()
	}
	command.WaitDelay = 2 * time.Second
}
