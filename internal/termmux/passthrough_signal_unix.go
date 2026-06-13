//go:build !windows

package termmux

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

// signalResult communicates a signal event from the signal watcher to
// the passthrough main loop.
type signalResult struct {
	reason ExitReason
	err    error
}

// watchSignals listens for SIGINT, SIGQUIT, and SIGTSTP during passthrough
// and forwards them to the child process via signalChild. It sends results
// to resultCh:
//   - SIGINT/SIGQUIT: forwarded to child, no result sent (the signal
//     reaches the child which may or may not exit)
//   - SIGTSTP: forwarded to child, then sends ExitSuspended so passthrough
//     can exit cleanly and the parent can resume the child later
//
// The caller must defer-call the returned cancel function to stop signal
// notification and prevent goroutine leaks.
func watchSignals(ctx context.Context, resultCh chan<- signalResult, signalChild func(sig string) error) context.CancelFunc {
	ch := make(chan os.Signal, 3)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTSTP)

	sigCtx, sigCancel := context.WithCancel(ctx)

	go func() {
		defer signal.Stop(ch)
		for {
			select {
			case <-sigCtx.Done():
				return
			case sig := <-ch:
				name := signalName(sig)
				if signalChild != nil {
					_ = signalChild(name)
				}
				if sig == syscall.SIGTSTP {
					select {
					case resultCh <- signalResult{ExitSuspended, nil}:
					case <-sigCtx.Done():
					}
					return
				}
			}
		}
	}()

	return sigCancel
}

// signalName maps a syscall.Signal to its string name for use with
// pty.Process.Signal.
func signalName(sig os.Signal) string {
	switch sig {
	case syscall.SIGINT:
		return "SIGINT"
	case syscall.SIGQUIT:
		return "SIGQUIT"
	case syscall.SIGTSTP:
		return "SIGTSTP"
	default:
		return sig.String()
	}
}
