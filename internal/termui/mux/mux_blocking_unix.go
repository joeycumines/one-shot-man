//go:build !windows

package mux

import "syscall"

// ensureBlockingFd clears the O_NONBLOCK flag from a file descriptor using
// fcntl. This is needed because go-prompt, BubbleTea's cancelreader, or
// other terminal libraries may leave stdin in non-blocking mode. Go's
// os.File.Read() does NOT handle EAGAIN for TTY file descriptors — it
// surfaces the error directly — so we must ensure blocking mode before
// reading in RunPassthrough.
//
// Returns the original flags so the caller can restore them.
func ensureBlockingFd(fd int) (origFlags int, err error) {
	origFlags, err = fcntlGetFlags(fd)
	if err != nil {
		return 0, err
	}
	if origFlags&syscall.O_NONBLOCK != 0 {
		// Clear O_NONBLOCK.
		if err := fcntlSetFlags(fd, origFlags&^syscall.O_NONBLOCK); err != nil {
			return origFlags, err
		}
	}
	return origFlags, nil
}

// restoreBlockingFd restores the file descriptor flags saved by
// ensureBlockingFd. No-op if origFlags is 0 (nothing to restore).
func restoreBlockingFd(fd int, origFlags int) {
	if origFlags == 0 {
		return
	}
	_ = fcntlSetFlags(fd, origFlags)
}

// fcntlGetFlags wraps fcntl(fd, F_GETFL, 0).
func fcntlGetFlags(fd int) (int, error) {
	flags, _, errno := syscall.Syscall(syscall.SYS_FCNTL, uintptr(fd), uintptr(syscall.F_GETFL), 0)
	if errno != 0 {
		return 0, errno
	}
	return int(flags), nil
}

// fcntlSetFlags wraps fcntl(fd, F_SETFL, flags).
func fcntlSetFlags(fd int, flags int) error {
	_, _, errno := syscall.Syscall(syscall.SYS_FCNTL, uintptr(fd), uintptr(syscall.F_SETFL), uintptr(flags))
	if errno != 0 {
		return errno
	}
	return nil
}
