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
func setProcAttr(cmd *osexec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

type unixProcessTree struct{}

func newProcessTree() (processTree, error) {
	return unixProcessTree{}, nil
}

func (unixProcessTree) attach(*osexec.Cmd) error {
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

func signalProcessGroup(cmd *osexec.Cmd, sig syscall.Signal) error {
	alive, err := processGroupLeaderAlive(cmd)
	if err != nil || !alive {
		return err
	}
	err = syscall.Kill(-cmd.Process.Pid, sig)
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

func (unixProcessTree) kill(cmd *osexec.Cmd) error {
	return signalProcessGroup(cmd, syscall.SIGKILL)
}

func (unixProcessTree) close(cmd *osexec.Cmd) error {
	return closeProcessGroup(cmd, syscall.Kill)
}

func closeProcessGroup(cmd *osexec.Cmd, kill func(pid int, signal syscall.Signal) error) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	if err := kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		return fmt.Errorf("kill process group: %w", err)
	}
	return nil
}

func processExitSignal(err error) string {
	var exitErr *osexec.ExitError
	if !errors.As(err, &exitErr) {
		return ""
	}
	status, ok := exitErr.Sys().(syscall.WaitStatus)
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

// signalProcess delivers the resolved signal to the child's process group,
// mirroring the tree cancellation semantics.
func signalProcess(cmd *osexec.Cmd, _ processTree, sig os.Signal) error {
	return signalProcessGroup(cmd, sig.(syscall.Signal))
}
