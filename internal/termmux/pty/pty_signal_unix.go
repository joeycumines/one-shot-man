//go:build !windows

package pty

import (
	"os"
	"syscall"
)

var extraSignals = map[string]os.Signal{
	"SIGSTOP": syscall.SIGSTOP,
	"SIGCONT": syscall.SIGCONT,
}
