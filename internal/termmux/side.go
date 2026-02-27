package termmux

import "fmt"

// Side represents which process currently owns the terminal.
type Side int

const (
	// SideOsm means osm's TUI owns the terminal.
	SideOsm Side = iota
	// SideClaude means the child PTY (e.g. Claude) owns the terminal.
	SideClaude
)

// ExitReason describes why RunPassthrough returned.
type ExitReason int

const (
	// ExitToggle means the user pressed the toggle key.
	ExitToggle ExitReason = iota
	// ExitChildExit means the child process exited (EOF on PTY read).
	ExitChildExit
	// ExitContext means the context was cancelled.
	ExitContext
	// ExitError means an I/O error occurred.
	ExitError
)

// String returns a human-readable exit reason name.
func (r ExitReason) String() string {
	switch r {
	case ExitToggle:
		return "toggle"
	case ExitChildExit:
		return "child-exit"
	case ExitContext:
		return "context"
	case ExitError:
		return "error"
	default:
		return fmt.Sprintf("unknown(%d)", int(r))
	}
}
