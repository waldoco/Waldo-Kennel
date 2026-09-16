//go:build !windows

package codexappserver

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

// A dedicated process group makes the app-server and every command it launches
// one owned cancellation unit. Killing only the app-server can orphan the shell
// whose effects Stop is meant to halt.
func configureAppServerProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func killAppServerProcessTree(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	if errors.Is(err, os.ErrProcessDone) || errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return err
}
