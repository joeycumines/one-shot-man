//go:build windows

package exec

import osexec "os/exec"

// setProcAttr is a no-op on Windows (no process group support via SysProcAttr).
func setProcAttr(cmd *osexec.Cmd) {}

// killProcess kills the process on Windows using Process.Kill().
func killProcess(cmd *osexec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	return cmd.Process.Kill()
}
