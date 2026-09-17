//go:build windows

package codexappserver

import "os/exec"

func configureAppServerProcess(_ *exec.Cmd) {}

func killAppServerProcessTree(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	return cmd.Process.Kill()
}
