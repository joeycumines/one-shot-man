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

// signalFromName resolves a signal name to an os.Signal, accepting the
// "SIG"-prefixed and bare forms. POSIX-only constants live in this
// unix-tagged file so the package still builds on Windows and Plan 9.
func signalFromName(name string) os.Signal {
	if sig, ok := map[string]os.Signal{
		"SIGHUP": syscall.SIGHUP, "SIGINT": syscall.SIGINT, "SIGQUIT": syscall.SIGQUIT,
		"SIGTERM": syscall.SIGTERM, "SIGUSR1": syscall.SIGUSR1, "SIGUSR2": syscall.SIGUSR2,
	}[name]; ok {
		return sig
	}
	if sig, ok := map[string]os.Signal{
		"HUP": syscall.SIGHUP, "INT": syscall.SIGINT, "QUIT": syscall.SIGQUIT,
		"TERM": syscall.SIGTERM, "USR1": syscall.SIGUSR1, "USR2": syscall.SIGUSR2,
	}[name]; ok {
		return sig
	}
	return nil
}

// signalProcess delivers the resolved signal to the child's process group,
// mirroring killProcess's tree semantics.
func signalProcess(cmd *osexec.Cmd, sig os.Signal) error {
	if cmd.Process == nil {
		return nil
	}
	return syscall.Kill(-cmd.Process.Pid, sig.(syscall.Signal))
}
