//go:build unix

package tview

import (
	"os"
	"syscall"
)

// defaultSignals are the signals that should stop the TUI application on Unix.
var defaultSignals = []os.Signal{
	syscall.SIGINT,
	syscall.SIGTERM,
	syscall.SIGQUIT,
}
