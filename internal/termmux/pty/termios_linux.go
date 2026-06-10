//go:build linux

package pty

import "golang.org/x/sys/unix"

const (
	tcgets  = unix.TCGETS
	tcsets  = unix.TCSETS
	tiocspgrp = unix.TIOCSPGRP
)
