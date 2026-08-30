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
	// ExitSuspended means the child process was stopped (SIGTSTP) and
	// passthrough exited so the parent can resume the child later.
	ExitSuspended
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
	case ExitSuspended:
		return "suspended"
	default:
		return fmt.Sprintf("unknown(%d)", int(r))
	}
}
