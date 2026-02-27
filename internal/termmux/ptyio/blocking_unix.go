//go:build !windows

package ptyio

import "syscall"

// UnixBlockingGuard clears O_NONBLOCK via fcntl.
type UnixBlockingGuard struct{}

var _ BlockingGuard = UnixBlockingGuard{}

func (UnixBlockingGuard) EnsureBlocking(fd int) (origFlags int, err error) {
	origFlags, err = fcntlGetFlags(fd)
	if err != nil {
		return 0, err
	}
	if origFlags&syscall.O_NONBLOCK != 0 {
		if err := fcntlSetFlags(fd, origFlags&^syscall.O_NONBLOCK); err != nil {
			return origFlags, err
		}
	}
	return origFlags, nil
}

func (UnixBlockingGuard) Restore(fd int, origFlags int) {
	if origFlags == 0 {
		return
	}
	_ = fcntlSetFlags(fd, origFlags)
}

func fcntlGetFlags(fd int) (int, error) {
	flags, _, errno := syscall.Syscall(syscall.SYS_FCNTL, uintptr(fd), uintptr(syscall.F_GETFL), 0)
	if errno != 0 {
		return 0, errno
	}
	return int(flags), nil
}

func fcntlSetFlags(fd int, flags int) error {
	_, _, errno := syscall.Syscall(syscall.SYS_FCNTL, uintptr(fd), uintptr(syscall.F_SETFL), uintptr(flags))
	if errno != 0 {
		return errno
	}
	return nil
}
