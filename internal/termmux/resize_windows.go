//go:build windows

package termmux

import (
	"context"
	"time"
)

// Windows has no SIGWINCH equivalent. The console does not signal
// resize events to the process, so we poll the terminal dimensions
// at a fixed interval and invoke the callback when they change.
// This is the same approach used by golang.org/x/term's Windows
// resize detection and by major terminal emulators on Windows.

const resizePollInterval = 250 * time.Millisecond

// watchResize polls the terminal size and calls fn with the new
// dimensions whenever they change. It exits cleanly when ctx is
// cancelled.
func watchResize(ctx context.Context, termFd int, ts interface {
	GetSize(fd int) (width, height int, err error)
}, fn func(rows, cols int)) {
	// Capture the initial size so we only call fn on actual changes.
	lastW, lastH := -1, -1
	if w, h, err := ts.GetSize(termFd); err == nil {
		lastW, lastH = w, h
		fn(h, w)
	}

	ticker := time.NewTicker(resizePollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w, h, err := ts.GetSize(termFd)
			if err != nil {
				continue
			}
			if w != lastW || h != lastH {
				lastW, lastH = w, h
				fn(h, w)
			}
		}
	}
}
