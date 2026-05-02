package termmux

import (
	"context"
	"errors"
	"io"
	"runtime"
	"syscall"
)

// forwardConfig configures the stdin→PTY forwarding loop used by both
// CaptureSession.Passthrough and SessionManager.Passthrough.
type forwardConfig struct {
	// Stdin is the user's terminal input.
	Stdin io.Reader
	// Writer is the PTY input writer (destination for forwarded bytes).
	Writer io.Writer
	// ToggleKey is the byte value that exits passthrough.
	ToggleKey byte
	// PreProcess is an optional callback invoked for each read chunk before
	// toggle-key scanning. It may modify the data (e.g., SGR mouse filtering)
	// and return the filtered data, a "clicked" flag, and any partial bytes
	// to carry over to the next read. If clicked is true, the forwarding loop
	// exits with ExitToggle immediately.
	PreProcess func(data []byte, carry []byte) (filtered []byte, newCarry []byte, clicked bool)
}

// forwardResult is the exit outcome of a forwarding loop.
type forwardResult struct {
	reason ExitReason
	err    error
}

// forwardStdin runs the stdin→PTY forwarding loop. It reads from stdin,
// applies optional pre-processing (SGR mouse filtering), scans for the
// toggle key, and writes data to the PTY writer.
//
// The loop exits when one of these conditions occurs:
//   - Toggle key is found in the input → ExitToggle
//   - PreProcess reports a click → ExitToggle
//   - Write to PTY fails → ExitError
//   - Read from stdin returns a permanent error (not EAGAIN/EWOULDBLOCK) → ExitError
//   - Read from stdin returns io.EOF → the goroutine returns (caller detects via other signal)
//   - fwdCtx is cancelled → goroutine returns silently
//
// The result is sent to resultCh. If resultCh is nil, the goroutine runs
// without reporting results (useful for fire-and-forget scenarios).
func forwardStdin(fwdCtx context.Context, resultCh chan<- forwardResult, cfg forwardConfig) {
	buf := make([]byte, 4096)
	var carry []byte // carry-over for partial SGR mouse prefixes

	for {
		select {
		case <-fwdCtx.Done():
			return
		default:
		}

		n, readErr := cfg.Stdin.Read(buf)
		// Process data first: io.Reader contract allows n > 0
		// alongside a non-nil error (commonly io.EOF).
		if n > 0 {
			data := buf[:n]

			// Pre-processing hook (e.g., SGR mouse filtering).
			if cfg.PreProcess != nil {
				filtered, newCarry, clicked := cfg.PreProcess(data, carry)
				if clicked {
					// Write any data before the click.
					if len(filtered) > 0 {
						writeOrLog(cfg.Writer, filtered, "pre-toggle-click")
					}
					if resultCh != nil {
						resultCh <- forwardResult{ExitToggle, nil}
					}
					return
				}
				data = filtered
				// Deep-copy carry: newCarry may alias the shared buf, which
				// will be overwritten by the next Read. We must copy to a
				// fresh allocation to avoid corruption.
				if len(newCarry) > 0 {
					carry = append([]byte(nil), newCarry...)
				} else {
					carry = nil
				}
			}

			// Toggle key scan.
			for i := 0; i < len(data); i++ {
				if data[i] == cfg.ToggleKey {
					if i > 0 {
						writeOrLog(cfg.Writer, data[:i], "pre-toggle-key")
					}
					if resultCh != nil {
						resultCh <- forwardResult{ExitToggle, nil}
					}
					return
				}
			}

			// Forward all bytes to the PTY.
			if _, writeErr := cfg.Writer.Write(data); writeErr != nil {
				if fwdCtx.Err() != nil {
					return
				}
				if resultCh != nil {
					resultCh <- forwardResult{ExitError, writeErr}
				}
				return
			}
		}

		// Handle read error after processing any data returned
		// alongside it (io.Reader contract: n > 0, err != nil).
		if readErr != nil {
			if fwdCtx.Err() != nil {
				return
			}
			// Defense-in-depth: retry on EAGAIN even after
			// EnsureBlocking, in case another goroutine re-set
			// O_NONBLOCK.
			if errors.Is(readErr, syscall.EAGAIN) || errors.Is(readErr, syscall.EWOULDBLOCK) {
				runtime.Gosched()
				continue
			}
			// Stdin EOF is normal (reader exhausted). Stop forwarding
			// and let other goroutines (child exit, context cancel)
			// determine the exit reason.
			if errors.Is(readErr, io.EOF) {
				return
			}
			if resultCh != nil {
				resultCh <- forwardResult{ExitError, readErr}
			}
			return
		}
	}
}
