//go:build unix

package exec

import (
	"os"
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

// signalProcess delivers the named POSIX signal ("SIGTERM", "SIGINT", ...)
// to the child's process group, mirroring killProcess's tree semantics. A
// signal without the SIG prefix is accepted, as os.SignalByName accepts both
// forms.
func signalProcess(cmd *osexec.Cmd, name string) error {
	if cmd.Process == nil {
		return nil
	}
	sig := signalFromName(name)
	if sig == nil {
		return os.ErrInvalid
	}
	return syscall.Kill(-cmd.Process.Pid, sig.(syscall.Signal))
}
