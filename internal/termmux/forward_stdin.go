package termmux

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"runtime"
	"syscall"
	"time"

	uv "github.com/charmbracelet/ultraviolet"
)

// forwardJoinGrace bounds how long a caller waits for the stdin forwarder to
// finish after cancelling it.
//
// Cancellation cannot always interrupt the blocking read. On the select-based
// platforms ultraviolet routes anything that is not an *os.File, and any
// *os.File whose descriptor is at or above FD_SETSIZE, to a fallback reader
// whose Cancel cannot un-park the read. Waiting on such a forwarder forever
// turns a leaked goroutine into a hang, so the join is best-effort.
// Abandoning it is safe: the forwarder checks the cancelled context before
// every write and every result send, so it cannot touch the caller's writer or
// channels after the caller returns.
const forwardJoinGrace = 250 * time.Millisecond

// joinForwarder waits for the stdin forwarder to exit after cancellation,
// giving up after forwardJoinGrace. A false return means the forwarder is
// still blocked in an uninterruptible read and was abandoned.
func joinForwarder(forwardDone <-chan struct{}) bool {
	timer := time.NewTimer(forwardJoinGrace)
	defer timer.Stop()
	select {
	case <-forwardDone:
		return true
	case <-timer.C:
		slog.Debug("stdin forwarder did not exit after cancel",
			"waitedMs", forwardJoinGrace.Milliseconds())
		return false
	}
}

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

func sendForwardResult(ctx context.Context, resultCh chan<- forwardResult, result forwardResult) {
	if resultCh == nil {
		return
	}
	select {
	case resultCh <- result:
	case <-ctx.Done():
	}
}

// newForwardReader wraps the terminal input with the platform-aware
// cancel-reader used by BubbleTea/U.V. The wrapper watches fwdCtx while the
// forward loop is blocked in Read, then closes only non-file fallback readers
// that cannot interrupt their own read. Shared os.Stdin is never closed.
func newForwardReader(ctx context.Context, input io.Reader) (io.Reader, func()) {
	if input == nil {
		return nil, func() {}
	}

	reader := input
	var cancelReader interface {
		io.Reader
		Cancel() bool
		Close() error
	}
	if cr, err := uv.NewCancelReader(input); err == nil && cr != nil {
		reader = cr
		cancelReader = cr
	}

	stopWatcher := make(chan struct{})
	watcherDone := make(chan struct{})
	go func() {
		defer close(watcherDone)
		select {
		case <-ctx.Done():
			canceled := false
			if cancelReader != nil {
				canceled = cancelReader.Cancel()
			}
			if !canceled {
				if closer, ok := input.(io.Closer); ok {
					// Do not close a shared file/terminal. File-backed readers
					// use ultraviolet's native cancellation when available.
					if _, isFile := input.(interface{ Fd() uintptr }); !isFile {
						_ = closer.Close()
					}
				}
			}
		case <-stopWatcher:
			return
		}
		// Release the cancel reader's own descriptors (epoll fd, cancel pipe)
		// here rather than in cleanup: this goroutine is guaranteed to run,
		// whereas cleanup only runs when the forwarder is joined. A forwarder
		// abandoned by joinForwarder would otherwise keep them open for the
		// life of the process.
		if cancelReader != nil {
			_ = cancelReader.Close()
		}
	}()

	cleanup := func() {
		close(stopWatcher)
		<-watcherDone
	}
	return reader, cleanup
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
	reader, cleanupReader := newForwardReader(fwdCtx, cfg.Stdin)
	defer cleanupReader()
	if reader == nil {
		return
	}
	buf := make([]byte, PassthroughReadBufferSize)
	var carry []byte // carry-over for partial SGR mouse prefixes

	for {
		select {
		case <-fwdCtx.Done():
			return
		default:
		}

		n, readErr := reader.Read(buf)
		// A cancel may race with a successful read. Never forward bytes after
		// passthrough has begun its terminal teardown.
		if fwdCtx.Err() != nil {
			return
		}
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
						if err := writeOrLog(cfg.Writer, filtered, "pre-toggle-click"); err != nil {
							if resultCh != nil {
								sendForwardResult(fwdCtx, resultCh, forwardResult{ExitError, err})
							}
							return
						}
					}
					if resultCh != nil {
						sendForwardResult(fwdCtx, resultCh, forwardResult{ExitToggle, nil})
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
						if err := writeOrLog(cfg.Writer, data[:i], "pre-toggle-key"); err != nil {
							if resultCh != nil {
								sendForwardResult(fwdCtx, resultCh, forwardResult{ExitError, err})
							}
							return
						}
					}
					if resultCh != nil {
						sendForwardResult(fwdCtx, resultCh, forwardResult{ExitToggle, nil})
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
					sendForwardResult(fwdCtx, resultCh, forwardResult{ExitError, writeErr})
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
				sendForwardResult(fwdCtx, resultCh, forwardResult{ExitError, readErr})
			}
			return
		}
	}
}
