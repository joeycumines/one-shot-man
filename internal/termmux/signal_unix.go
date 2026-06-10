//go:build unix

package termmux

import (
	"os/signal"
	"syscall"
)

func init() {
	// Ignore SIGTTOU and SIGTTIN so that termmux and its PTY children
	// are not stopped by the kernel when they interact with the terminal
	// from a background process group. This commonly occurs when:
	//   - The child process calls tcsetattr (e.g., shells setting raw mode)
	//   - The termmux passthrough writes to the real terminal
	//   - The BubbleTea runtime enters/exits raw mode
	//
	// Without this, programs like vim, less, and osm super document
	// would hang on startup due to SIGTTOU being delivered.
	signal.Ignore(syscall.SIGTTOU, syscall.SIGTTIN)
}
