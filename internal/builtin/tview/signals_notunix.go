//go:build !unix

package tview

import (
	"os"
)

// defaultSignals are the signals that should stop the TUI application on Windows.
// Windows only supports os.Interrupt (Ctrl+C).
var defaultSignals = []os.Signal{
	os.Interrupt,
}
