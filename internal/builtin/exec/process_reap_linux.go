//go:build linux

package exec

import (
	"errors"
	osexec "os/exec"

	"golang.org/x/sys/unix"
)

// waitProcessBeforeReap observes the direct child's exit without reaping it.
// The zombie leader pins the process-group ID until descendants are closed.
func waitProcessBeforeReap(cmd *osexec.Cmd) (bool, error) {
	if cmd == nil || cmd.Process == nil {
		return false, nil
	}
	var info unix.Siginfo
	for {
		err := unix.Waitid(unix.P_PID, cmd.Process.Pid, &info, unix.WEXITED|unix.WNOWAIT, nil)
		if err == nil {
			return true, nil
		}
		if errors.Is(err, unix.EINTR) {
			continue
		}
		return false, err
	}
}
