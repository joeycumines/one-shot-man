//go:build windows

package termmux

import (
	"context"
	"os"
	"os/signal"
)

// signalResult communicates a signal event from the signal watcher to
// the passthrough main loop. Defined here for compilation on Windows;
// the Unix version is in passthrough_signal_unix.go.
type signalResult struct {
	reason ExitReason
	err    error
}

// watchSignals listens for OS interrupt signals during passthrough on Windows
// and forwards them to the child process via signalChild.
func watchSignals(ctx context.Context, resultCh chan<- signalResult, signalChild func(sig string) error) context.CancelFunc {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)

	sigCtx, sigCancel := context.WithCancel(ctx)

	go func() {
		defer signal.Stop(ch)
		for {
			select {
			case <-sigCtx.Done():
				return
			case <-ch:
				if signalChild != nil {
					if err := signalChild("SIGINT"); err != nil {
						select {
						case resultCh <- signalResult{ExitError, err}:
						case <-sigCtx.Done():
						}
						return
					}
				}
			}
		}
	}()

	return sigCancel
}
