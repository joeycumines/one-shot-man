package termmux

import "fmt"

// ExitReason describes why RunPassthrough returned.
type ExitReason int

const (
	// ExitToggle means the user pressed the toggle key.
	ExitToggle ExitReason = iota
	// ExitChildExit means the attached session exited (EOF on PTY read).
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
