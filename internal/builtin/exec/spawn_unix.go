//go:build unix

package exec

import (
	osexec "os/exec"
	"syscall"
)

// setProcAttr configures the command to create a new process group on Unix.
// This allows killProcess to kill the entire tree via negative PID.
func setProcAttr(cmd *osexec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killProcess sends SIGKILL to the process group on Unix.
func killProcess(cmd *osexec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}
