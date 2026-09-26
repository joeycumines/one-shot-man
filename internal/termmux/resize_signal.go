package termmux

import (
	"context"
	"os"
)

// watchResizeSignalLoop is the signal-driven resize loop shared by the
// platform watcher and deterministic tests. The caller owns the signal
// channel and is responsible for registering and stopping signal delivery.
func watchResizeSignalLoop(
	ctx context.Context,
	termFd int,
	ts interface {
		GetSize(fd int) (width, height int, err error)
	},
	signals <-chan os.Signal,
	fn func(rows, cols int),
) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-signals:
			w, h, err := ts.GetSize(termFd)
			if err != nil {
				continue
			}
			fn(h, w)
		}
	}
}
