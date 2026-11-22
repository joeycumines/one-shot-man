//go:build unix

package exec

import (
	"errors"
	"fmt"
	"os"
	osexec "os/exec"
	"syscall"
)

// setProcAttr configures the command to create a new process group on Unix.
// This allows process-tree cancellation to terminate descendants via the
// negative process-group ID.
func setProcAttr(cmd *osexec.Cmd, _ bool) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func newProcessTree() (processTree, error) {
	return &unixProcessTree{}, nil
}

func (*unixProcessTree) attach(*osexec.Cmd) error {
	return nil
}

func processGroupLeaderAlive(cmd *osexec.Cmd) (bool, error) {
	if cmd == nil || cmd.Process == nil {
		return false, nil
	}
	if err := cmd.Process.Signal(syscall.Signal(0)); err != nil {
		if errors.Is(err, os.ErrProcessDone) {
			return false, nil
		}
		return false, fmt.Errorf("probe process group leader: %w", err)
	}
	return true, nil
}

// signalProcessGroup runs only while the tree owner holds its signal gate.
// The caller must prevent Cmd.Wait from beginning until this call returns.
func signalProcessGroup(cmd *osexec.Cmd, sig syscall.Signal, kill func(int, syscall.Signal) error) error {
	alive, err := processGroupLeaderAlive(cmd)
	if err != nil || !alive {
		return err
	}
	err = kill(-cmd.Process.Pid, sig)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	if errors.Is(err, syscall.EPERM) {
		// A process can exit between the liveness probe and the group
		// signal. Re-check the leader before surfacing a permission error.
		alive, checkErr := processGroupLeaderAlive(cmd)
		if checkErr != nil {
			return errors.Join(err, checkErr)
		}
		if !alive {
			return nil
		}
	}
	return err
}

func (t *unixProcessTree) kill(cmd *osexec.Cmd) error {
	return t.signalGroup(cmd, syscall.SIGKILL, syscall.Kill)
}

func (t *unixProcessTree) close(cmd *osexec.Cmd) error {
	return t.closeGroup(cmd, syscall.Kill)
}

func killProcessGroup(cmd *osexec.Cmd, kill func(pid int, signal syscall.Signal) error) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	if err := kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		return fmt.Errorf("kill process group: %w", err)
	}
	return nil
}

// exitStatusUnclassified reports whether a recorded exit status cannot be
// presented to JS as one.
//
// A signalled child's ExitCode is -1, so a negative code on its own means
// nothing; only a negative code with no signal is genuinely unclassified, and
// that must reject rather than masquerade as the command's own status.
func exitStatusUnclassified(code int, waitErr error) bool {
	return code < 0 && processExitSignal(waitErr) == ""
}

func processExitSignal(err error) string {
	var exitErr *osexec.ExitError
	if !errors.As(err, &exitErr) {
		return ""
	}
	if exitErr.ProcessState == nil {
		// Defensive: a synthesized ExitError has no process state to read a
		// signal from. No signal is reported for it, and callers treat the
		// child as having no signal.
		return ""
	}
	return processStateSignal(exitErr.ProcessState)
}

// processStateSignal names the signal that terminated a reaped child, or ""
// when it exited normally. Reading it at reap time keeps the name available
// even when cmd.Wait returned a context error whose chain hides the ExitError.
func processStateSignal(ps *os.ProcessState) string {
	if ps == nil {
		return ""
	}
	status, ok := ps.Sys().(syscall.WaitStatus)
	if !ok || !status.Signaled() {
		return ""
	}
	switch signal := status.Signal(); signal {
	case syscall.SIGHUP:
		return "SIGHUP"
	case syscall.SIGINT:
		return "SIGINT"
	case syscall.SIGQUIT:
		return "SIGQUIT"
	case syscall.SIGKILL:
		return "SIGKILL"
	case syscall.SIGTERM:
		return "SIGTERM"
	case syscall.SIGUSR1:
		return "SIGUSR1"
	case syscall.SIGUSR2:
		return "SIGUSR2"
	default:
		return fmt.Sprintf("signal %d", signal)
	}
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

// signal delivers the resolved signal under the same gate as cancellation
// and reaping, so a former process-group ID cannot be signalled after Wait.
func (t *unixProcessTree) signal(cmd *osexec.Cmd, sig os.Signal) error {
	return t.signalGroup(cmd, sig.(syscall.Signal), syscall.Kill)
}
