//go:build !windows

package termmux

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

// watchResize listens for SIGWINCH signals and calls fn with the
// current terminal dimensions whenever the terminal is resized.
// It exits cleanly when ctx is cancelled.
func watchResize(ctx context.Context, termFd int, ts interface {
	GetSize(fd int) (width, height int, err error)
}, fn func(rows, cols int)) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)
	defer signal.Stop(ch)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ch:
			w, h, err := ts.GetSize(termFd)
			if err != nil {
				continue
			}
			fn(h, w)
		}
	}
}
