package termmux

import (
	"io"

	"github.com/joeycumines/one-shot-man/internal/termmux/ptyio"
	"github.com/joeycumines/one-shot-man/internal/termmux/statusbar"
)

// NoTerminal is the sentinel value for TerminalIO.TermFd when no
// controlling terminal is available. Use this instead of 0 to avoid
// ambiguity — the zero value (0) is os.Stdin.Fd() on most systems.
const NoTerminal = -1

// TerminalIO holds the raw terminal I/O plumbing for passthrough mode.
type TerminalIO struct {
	// Stdin is the user's terminal input (typically os.Stdin).
	Stdin io.Reader
	// Stdout is the user's terminal output (typically os.Stdout).
	Stdout io.Writer
	// TermFd is the file descriptor for the controlling terminal.
	// Use NoTerminal (-1) if no terminal is available. The zero value (0)
	// is os.Stdin.Fd() on most systems — set explicitly to avoid ambiguity.
	TermFd int
	// BlockingGuard abstracts fd blocking mode management.
	BlockingGuard ptyio.BlockingGuard
}

// PassthroughOptions holds behavioral configuration for passthrough mode.
type PassthroughOptions struct {
	// ToggleKey is the byte value of the key that exits passthrough (e.g., 0x1D for Ctrl+]).
	ToggleKey byte
	// TermState abstracts terminal state operations (MakeRaw, Restore, GetSize).
	TermState ptyio.TermState
}

// ResizeConfig holds resize and screen restoration plumbing used by
// SessionManager.Passthrough (not CaptureSession.Passthrough).
type ResizeConfig struct {
	// ResizeFn, when non-nil, is called after terminal resize events
	// to propagate the new dimensions to the child process. The rows
	// parameter accounts for the status bar height.
	ResizeFn func(rows, cols uint16) error

	// RestoreScreen controls the initial display when entering
	// passthrough via SessionManager. When true, the active session's
	// VTerm screen is restored in-place (flicker-free re-entry).
	// When false, the screen is cleared (first swap).
	RestoreScreen bool
}

// UIConfig holds UI chrome configuration for passthrough mode.
type UIConfig struct {
	// StatusBar, when non-nil, reserves the last terminal row for a
	// persistent status line. A scroll region is set to constrain
	// child output, and SGR mouse events on the status bar row are
	// intercepted (click → ExitToggle).
	StatusBar *statusbar.StatusBar
}

// PassthroughConfig configures a passthrough session.
// Used by both CaptureSession.Passthrough() and SessionManager.Passthrough().
// It composes focused sub-types for each concern area.
type PassthroughConfig struct {
	TerminalIO
	PassthroughOptions
	ResizeConfig
	UIConfig
}
