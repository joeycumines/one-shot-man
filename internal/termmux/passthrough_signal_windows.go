//go:build windows

package termmux

import "context"

// signalResult communicates a signal event from the signal watcher to
// the passthrough main loop. Defined here for compilation on Windows;
// the Unix version is in passthrough_signal_unix.go.
type signalResult struct {
	reason ExitReason
	err    error
}

// watchSignals is a no-op on Windows — SIGINT, SIGQUIT, and SIGTSTP
// are not supported. Returns a no-op cancel function.
func watchSignals(_ context.Context, _ chan<- signalResult, _ func(sig string) error) context.CancelFunc {
	return func() {}
}
