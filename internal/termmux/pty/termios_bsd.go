//go:build freebsd || openbsd || netbsd || dragonfly

package pty

import "golang.org/x/sys/unix"

const (
	tcgets = unix.TIOCGETA
	tcsets = unix.TIOCSETA
)
