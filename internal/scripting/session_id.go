//go:build linux || darwin || freebsd || openbsd || netbsd

package scripting

import (
	"fmt"
	"os"

	"golang.org/x/term"
)

// getTerminalID returns a session ID based on the controlling terminal device.
// This provides stable session IDs per terminal window on POSIX systems.
func getTerminalID() string {
	// Check if standard input is a terminal.
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return ""
	}

	// Get the terminal device name using readlink on /proc/self/fd/{fd} or ttyname syscall
	// The most portable POSIX approach is to read the symlink /dev/fd/{fd}
	// or use the controlling terminal device /dev/tty
	name, err := os.Readlink(fmt.Sprintf("/dev/fd/%d", fd))
	if err != nil {
		// Fallback: try to open /dev/tty which represents the controlling terminal
		// Just return its path as identifier
		if _, err := os.Stat("/dev/tty"); err == nil {
			// Use /dev/tty but we need something more unique
			// Try ttyname via cgo or fall back to empty
			return ""
		}
		return ""
	}

	return name
}
