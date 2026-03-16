//go:build !windows

package pty

import (
	"os"
	"syscall"
)

// extraSignals maps signal names to os.Signal values that are only
// available on Unix-like platforms (not Windows).
var extraSignals = map[string]os.Signal{
	"SIGSTOP": syscall.SIGSTOP,
	"SIGCONT": syscall.SIGCONT,
}
