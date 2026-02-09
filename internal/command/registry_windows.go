//go:build windows

package command

import (
	"fmt"
	"os/exec"
)

// setupSysProcAttr is a no-op on Windows.
func (c *ScriptCommand) setupSysProcAttr(cmd *exec.Cmd) {
	// Windows does not use Setpgid or process groups in the same way
}

// killProcessGroup kills the entire process group for a command on Windows.
func (c *ScriptCommand) killProcessGroup(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}

	// Windows: use taskkill to terminate the process tree
	_ = exec.Command("taskkill", "/F", "/T", "/PID", fmt.Sprintf("%d", cmd.Process.Pid)).Run()
}
