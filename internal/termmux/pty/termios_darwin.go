//go:build darwin

package pty

import "golang.org/x/sys/unix"

const (
	tcgets  = unix.TIOCGETA
	tcsets  = unix.TIOCSETA
	tiocspgrp = unix.TIOCSPGRP
)
