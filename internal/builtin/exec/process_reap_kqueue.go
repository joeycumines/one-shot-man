//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package exec

import (
	"errors"
	osexec "os/exec"
	"syscall"
)

// waitProcessBeforeReap observes the direct child's exit without reaping it.
// kqueue's process filter leaves the zombie leader waitable, so its PID and
// process-group ID cannot be recycled before descendants are closed.
func waitProcessBeforeReap(cmd *osexec.Cmd) (bool, error) {
	if cmd == nil || cmd.Process == nil {
		return false, nil
	}

	kq, err := syscall.Kqueue()
	if err != nil {
		return false, err
	}
	defer syscall.Close(kq)

	change := syscall.Kevent_t{
		Ident:  uint64(cmd.Process.Pid),
		Filter: int16(syscall.EVFILT_PROC),
		Flags:  syscall.EV_ADD | syscall.EV_ENABLE,
		Fflags: uint32(syscall.NOTE_EXIT),
	}
	if _, err := syscall.Kevent(kq, []syscall.Kevent_t{change}, nil, nil); err != nil {
		if errors.Is(err, syscall.ESRCH) {
			// The child may have exited between Start and filter
			// registration. A successful signal-0 probe means the zombie
			// still pins its PID, so it is safe to treat that as observed.
			if alive, probeErr := processGroupLeaderAlive(cmd); probeErr == nil && alive {
				return true, nil
			}
		}
		return false, err
	}

	events := make([]syscall.Kevent_t, 1)
	for {
		n, err := syscall.Kevent(kq, nil, events, nil)
		if err != nil {
			if errors.Is(err, syscall.EINTR) {
				continue
			}
			return false, err
		}
		if n > 0 && events[0].Fflags&uint32(syscall.NOTE_EXIT) != 0 {
			return true, nil
		}
	}
}
