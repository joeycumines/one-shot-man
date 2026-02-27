//go:build !windows

package ptyio

import (
	"syscall"
	"unsafe"
)

type winsize struct {
	Row    uint16
	Col    uint16
	Xpixel uint16
	Ypixel uint16
}

// ResizePTY sets the window size of a PTY file descriptor using TIOCSWINSZ.
func ResizePTY(fd uintptr, rows, cols uint16) error {
	ws := winsize{Row: rows, Col: cols}
	_, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		fd,
		syscall.TIOCSWINSZ,
		uintptr(unsafe.Pointer(&ws)),
	)
	if errno != 0 {
		return errno
	}
	return nil
}
