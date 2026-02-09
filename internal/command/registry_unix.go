//go:build unix

package command

import (
	"os/exec"
	"syscall"
	"time"
)

// setupSysProcAttr configures Unix-specific process attributes.
func (c *ScriptCommand) setupSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true, // Create new process group
	}
}

// killProcessGroup kills the entire process group for a command on Unix.
func (c *ScriptCommand) killProcessGroup(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}

	// Unix: kill the entire process group
	// Negative PID means kill the entire process group
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
	// Give processes a moment to terminate gracefully
	time.Sleep(100 * time.Millisecond)
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}
