//go:build windows

package exec

import (
	"os"
	osexec "os/exec"
)

// setProcAttr is a no-op on Windows (no process group support via SysProcAttr).
func setProcAttr(cmd *osexec.Cmd) {}

// killProcess kills the process on Windows using Process.Kill().
func killProcess(cmd *osexec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	return cmd.Process.Kill()
}

// signalFromName resolves the POSIX-style signal names the JS surface
// accepts. Windows has no POSIX signal delivery: every known name maps to
// os.Interrupt as a dummy accepted os.Signal, and signalProcess — which is
// Process.Kill on this platform — provides the only (terminate-style)
// delivery anyway. Unknown names still return nil so a typo'd signal
// rejects with os.ErrInvalid instead of degrading into a kill.
func signalFromName(name string) os.Signal {
	switch name {
	case "SIGHUP", "HUP", "SIGINT", "INT", "SIGQUIT", "QUIT", "SIGTERM", "TERM",
		"SIGUSR1", "USR1", "SIGUSR2", "USR2":
		return os.Interrupt
	default:
		return nil
	}
}

// signalProcess is a Windows approximation: Process.Kill is the strongest
// available delivery.
func signalProcess(cmd *osexec.Cmd, sig os.Signal) error {
	if cmd.Process == nil {
		return nil
	}
	return cmd.Process.Kill()
}
