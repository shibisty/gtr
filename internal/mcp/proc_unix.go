//go:build !windows

package mcp

import (
	"os/exec"
	"syscall"
)

// setProcessGroup starts the command in a process group of its own, so
// killTree stops its children too.
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func killTree(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
		cmd.Process.Kill()
	}
}
