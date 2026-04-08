package termmux

import (
	"context"
	"errors"
	"io"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/joeycumines/one-shot-man/internal/termmux/pty"
	"github.com/joeycumines/one-shot-man/internal/termmux/ptyio"
	"github.com/joeycumines/one-shot-man/internal/termmux/statusbar"
)

// CaptureConfig configures a CaptureSession.
type CaptureConfig struct {
	// Name is an optional human-readable label for the session.
	Name string
	// Kind classifies the session. Defaults to SessionKindCapture.
	Kind SessionKind
	// Command is the executable path or name.
	Command string
	// Args are the command arguments.
	Args []string
	// Dir is the working directory (default: caller's CWD).
	Dir string
	// Env contains additional environment variables merged with os.Environ().
	Env map[string]string
	// Rows is the virtual terminal row count (default: 24).
	Rows int
	// Cols is the virtual terminal column count (default: 80).
	Cols int
	// DrainTimeout is the maximum time Close() waits for the reader loop
	// to finish after the PTY is closed. Defaults to 5 seconds.
	DrainTimeout time.Duration
}

// CaptureSession manages a PTY-attached command with real-time output capture.
// It is a simplified, standalone alternative to SessionManager for cases where
// only raw output forwarding is needed — no terminal multiplexing, toggle keys,
// status bar, or raw-mode management.
//
// Usage:
//
//	cs := termmux.NewCaptureSession(termmux.CaptureConfig{
//	    Command: "make",
//	    Args:    []string{"test"},
//	    Dir:     "/path/to/project",
//	})
//	if err := cs.Start(ctx); err != nil { ... }
//	// Consume raw output via cs.Reader().
//	exitCode, err := cs.Wait()
//	cs.Close()
//
// All methods are safe for concurrent use.
type CaptureSession struct {
	mu   sync.Mutex
	cfg  CaptureConfig
	proc *pty.Process

	// Lifecycle state.
	started  bool
	closed   bool
	paused   bool
	done     chan struct{} // closed when reader goroutine exits
	cancel   context.CancelFunc
	exitCode int
	exitErr  error

	// Terminal dimensions (may change via Resize).
	rows int
	cols int

	// Buffered reader for PTY output. Set during Start; used by
	// readerLoop which consumes from reader.Output() channel.
	reader       *ptyio.BufferedReader
	readerCancel context.CancelFunc // cancels BufferedReader.ReadLoop

	// outputCh streams raw PTY output for consumption by SessionManager
	// via the Reader() method. Each chunk is a copy of what the
	// BufferedReader produces. The channel is closed on EOF.
	outputCh chan []byte

	// Passthrough state: when passthroughActive is true, the readerLoop
	// also writes output chunks to passthroughOutput (typically os.Stdout).
	passthroughActive bool
	passthroughOutput io.Writer // set before activating passthrough
}

// NewCaptureSession creates a new capture session with the given configuration.
// The session is not started until Start is called.
func NewCaptureSession(cfg CaptureConfig) *CaptureSession {
	rows := cfg.Rows
	if rows <= 0 {
		rows = 24
	}
	cols := cfg.Cols
	if cols <= 0 {
		cols = 80
	}
	if cfg.DrainTimeout <= 0 {
		cfg.DrainTimeout = 5 * time.Second
	}
	return &CaptureSession{
		cfg:  cfg,
		done: make(chan struct{}),
		rows: rows,
		cols: cols,
	}
}

// Start spawns the command in a PTY and begins capturing output. The context
// controls the lifetime of the underlying process — cancelling it sends
// SIGKILL to the child. Start may be called only once; subsequent calls
// return an error.
func (cs *CaptureSession) Start(ctx context.Context) error {
	cs.mu.Lock()
	if cs.started {
		cs.mu.Unlock()
		return errors.New("capture: already started")
	}
	if cs.closed {
		cs.mu.Unlock()
		return errors.New("capture: session is closed")
	}
	cs.started = true
	cs.mu.Unlock()

	childCtx, cancel := context.WithCancel(ctx)
	proc, err := pty.Spawn(childCtx, pty.SpawnConfig{
		Command: cs.cfg.Command,
		Args:    cs.cfg.Args,
		Dir:     cs.cfg.Dir,
		Env:     cs.cfg.Env,
		Rows:    uint16(cs.rows),
		Cols:    uint16(cs.cols),
	})
	if err != nil {
		cancel()
		cs.mu.Lock()
		cs.started = false
		cs.mu.Unlock()
		return err
	}

	cs.mu.Lock()
	cs.proc = proc
	cs.cancel = cancel
	cs.mu.Unlock()

	// Start a buffered reader that wraps the PTY process file descriptor.
	// This provides a channel-based output stream that the readerLoop
	// consumes, allowing passthrough to route output to stdout without
	// racing on the PTY file descriptor. Uses the raw PTY fd (proc.File())
	// rather than proc itself because BufferedReader requires io.Reader
	// (Read([]byte)) while Process.Read returns (string, error).
	readerCtx, readerCancel := context.WithCancel(childCtx)
	cs.mu.Lock()
	cs.reader = ptyio.NewBufferedReader(proc.File(), 16)
	cs.readerCancel = readerCancel
	cs.outputCh = make(chan []byte, 16)
	cs.mu.Unlock()
	go cs.reader.ReadLoop(readerCtx)

	// Start the background reader that forwards PTY output to the
	// Reader() channel. The reader goroutine also captures exit status
	// before signaling completion, ensuring Wait() always returns the
	// correct exit code.
	go cs.readerLoop()

	return nil
}

// readerLoop consumes output from the BufferedReader channel and forwards
// chunks to the outputCh (for Reader() consumers like SessionManager).
// When passthroughActive is true, output is also forwarded to
// passthroughOutput (typically os.Stdout). After the channel closes
// (PTY EOF), it captures the process exit status and closes cs.done.
func (cs *CaptureSession) readerLoop() {
	defer close(cs.done)

	cs.mu.Lock()
	reader := cs.reader
	outputCh := cs.outputCh
	cs.mu.Unlock()

	defer func() {
		if outputCh != nil {
			close(outputCh)
		}
	}()

	// Drain all output from the BufferedReader into outputCh.
	for chunk := range reader.Output() {
		// Forward to Reader() channel for SessionManager consumption.
		// Non-blocking send avoids stalling the readerLoop which would
		// block the PTY and potentially deadlock the child process.
		if outputCh != nil {
			// Copy chunk to avoid aliasing with BufferedReader's buffer.
			cp := make([]byte, len(chunk))
			copy(cp, chunk)
			select {
			case outputCh <- cp:
			default:
			}
		}

		// During passthrough, also forward raw output to stdout.
		cs.mu.Lock()
		if cs.passthroughActive && cs.passthroughOutput != nil {
			writeOrLog(cs.passthroughOutput, chunk, "capture-passthrough-output")
		}
		cs.mu.Unlock()
	}

	// Capture exit status. proc.Wait() returns immediately here because
	// the process has already exited (Read returned error/EOF).
	code, err := cs.proc.Wait()
	cs.mu.Lock()
	cs.exitCode = code
	cs.exitErr = err
	cs.mu.Unlock()
	// done is closed by the deferred close(cs.done) after this returns.
}

// Interrupt sends SIGINT to the child process.
func (cs *CaptureSession) Interrupt() error {
	cs.mu.Lock()
	proc := cs.proc
	cs.mu.Unlock()
	if proc == nil {
		return errors.New("capture: not started")
	}
	return proc.Signal("SIGINT")
}

// Kill sends SIGKILL to the child process.
func (cs *CaptureSession) Kill() error {
	cs.mu.Lock()
	proc := cs.proc
	cs.mu.Unlock()
	if proc == nil {
		return errors.New("capture: not started")
	}
	return proc.Signal("SIGKILL")
}

// Pause sends SIGSTOP to the child process, suspending it. The process
// can be resumed with Resume(). On platforms that do not support SIGSTOP,
// Pause returns an error. Use IsPaused() to check the pause state.
func (cs *CaptureSession) Pause() error {
	cs.mu.Lock()
	proc := cs.proc
	if cs.paused {
		cs.mu.Unlock()
		return nil // already paused
	}
	cs.mu.Unlock()
	if proc == nil {
		return errors.New("capture: not started")
	}
	if err := proc.Signal("SIGSTOP"); err != nil {
		return err
	}
	cs.mu.Lock()
	cs.paused = true
	cs.mu.Unlock()
	return nil
}

// Resume sends SIGCONT to the child process, resuming it after a Pause().
// On platforms that do not support SIGCONT, Resume returns an error.
func (cs *CaptureSession) Resume() error {
	cs.mu.Lock()
	proc := cs.proc
	if !cs.paused {
		cs.mu.Unlock()
		return nil // not paused
	}
	cs.mu.Unlock()
	if proc == nil {
		return errors.New("capture: not started")
	}
	if err := proc.Signal("SIGCONT"); err != nil {
		return err
	}
	cs.mu.Lock()
	cs.paused = false
	cs.mu.Unlock()
	return nil
}

// IsPaused returns true if the child process is currently paused via Pause().
func (cs *CaptureSession) IsPaused() bool {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return cs.paused
}

// Resize changes the PTY dimensions. Returns an error if the session has
// not been started.
func (cs *CaptureSession) Resize(rows, cols int) error {
	if rows <= 0 || cols <= 0 {
		return errors.New("capture: rows and cols must be positive")
	}
	if rows > 65535 || cols > 65535 {
		return errors.New("capture: rows and cols must be <= 65535")
	}
	cs.mu.Lock()
	proc := cs.proc
	cs.mu.Unlock()
	if proc == nil {
		return errors.New("capture: not started")
	}
	// Resize PTY (delivers SIGWINCH to child).
	if err := proc.Resize(uint16(rows), uint16(cols)); err != nil {
		return err
	}
	cs.mu.Lock()
	cs.rows = rows
	cs.cols = cols
	cs.mu.Unlock()
	return nil
}

// Reader returns a channel that streams raw PTY output chunks. The channel
// is created during Start and closed when the child process exits (EOF).
// Returns nil if the session has not been started.
func (cs *CaptureSession) Reader() <-chan []byte {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return cs.outputCh
}

// Wait blocks until the child process exits and the output has been fully
// drained. Returns the exit code and any process error. Returns an error
// immediately if the session has not been started.
func (cs *CaptureSession) Wait() (int, error) {
	cs.mu.Lock()
	started := cs.started
	cs.mu.Unlock()
	if !started {
		return -1, errors.New("capture: not started")
	}
	<-cs.done // wait for reader loop to finish (all output captured)
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return cs.exitCode, cs.exitErr
}

// Done returns a channel that is closed when the child process exits and
// all output has been drained. Useful for non-blocking completion checks.
func (cs *CaptureSession) Done() <-chan struct{} {
	return cs.done
}

// Close terminates the child process (if running) and releases resources.
// Close is idempotent — subsequent calls return nil.
func (cs *CaptureSession) Close() error {
	cs.mu.Lock()
	if cs.closed {
		cs.mu.Unlock()
		return nil
	}
	cs.closed = true
	cancel := cs.cancel
	readerCancel := cs.readerCancel
	proc := cs.proc
	cs.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if readerCancel != nil {
		readerCancel()
	}
	if proc != nil {
		err := proc.Close()
		// Wait for reader loop to finish so all output is captured.
		// proc.Close() closes the PTY fd, which causes the BufferedReader's
		// ReadLoop to exit, which closes the output channel, which causes
		// readerLoop to exit. The timeout is a safety net for edge cases
		// where fd closure doesn't unblock immediately.
		select {
		case <-cs.done:
		case <-time.After(cs.cfg.DrainTimeout):
		}
		return err
	}
	return nil
}

// ExitCode returns the exit code of the child process. Only valid after
// Wait returns. Returns -1 if the process has not exited.
func (cs *CaptureSession) ExitCode() int {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if cs.proc == nil || cs.proc.IsAlive() {
		return -1
	}
	return cs.exitCode
}

// Pid returns the child process PID. Returns 0 if the session has not been
// started.
func (cs *CaptureSession) Pid() int {
	cs.mu.Lock()
	proc := cs.proc
	cs.mu.Unlock()
	if proc == nil {
		return 0
	}
	return proc.Pid()
}

// ExportConfig returns a copy of the session's creation configuration.
// This enables persistence of the command, arguments, and working directory
// for session restart scenarios.
func (cs *CaptureSession) ExportConfig() CaptureConfig {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cfg := cs.cfg
	// Deep-copy slices and maps to prevent mutation.
	if cs.cfg.Args != nil {
		cfg.Args = make([]string, len(cs.cfg.Args))
		copy(cfg.Args, cs.cfg.Args)
	}
	if cs.cfg.Env != nil {
		cfg.Env = make(map[string]string, len(cs.cfg.Env))
		for k, v := range cs.cfg.Env {
			cfg.Env[k] = v
		}
	}
	return cfg
}

// Write sends raw bytes to the child process's stdin via the PTY.
func (cs *CaptureSession) Write(data []byte) (int, error) {
	cs.mu.Lock()
	proc := cs.proc
	cs.mu.Unlock()
	if proc == nil {
		return 0, errors.New("capture: not started")
	}
	err := proc.Write(string(data))
	if err != nil {
		return 0, err
	}
	return len(data), nil
}

// WriteString sends string data to the child process's stdin via the PTY.
func (cs *CaptureSession) WriteString(data string) error {
	_, err := cs.Write([]byte(data))
	return err
}

// SendEOF sends EOF (Ctrl-D) to the child process via the PTY.
func (cs *CaptureSession) SendEOF() error {
	return cs.WriteString("\x04")
}

// PassthroughConfig configures a passthrough session.
// Used by both CaptureSession.Passthrough() and SessionManager.Passthrough().
type PassthroughConfig struct {
	// Stdin is the user's terminal input (typically os.Stdin).
	Stdin io.Reader
	// Stdout is the user's terminal output (typically os.Stdout).
	Stdout io.Writer
	// TermFd is the file descriptor for the controlling terminal (-1 if unavailable).
	TermFd int
	// ToggleKey is the byte value of the key that exits passthrough (e.g., 0x1D for Ctrl+]).
	ToggleKey byte
	// TermState abstracts terminal state operations (MakeRaw, Restore, GetSize).
	TermState ptyio.TermState
	// BlockingGuard abstracts fd blocking mode management.
	BlockingGuard ptyio.BlockingGuard

	// --- Fields below are used only by SessionManager.Passthrough() ---

	// StatusBar, when non-nil, reserves the last terminal row for a
	// persistent status line. A scroll region is set to constrain
	// child output, and SGR mouse events on the status bar row are
	// intercepted (click → ExitToggle).
	StatusBar *statusbar.StatusBar

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

// Passthrough enters raw terminal passthrough mode for this CaptureSession.
// It enables direct forwarding of child output to stdout and stdin to the
// child process, with toggle key detection for exit.
//
// The caller is responsible for ensuring the terminal is not in use by
// BubbleTea or any other consumer (typically achieved by calling
// ReleaseTerminal before this function and RestoreTerminal after).
//
// Unlike SessionManager.Passthrough, this method:
//   - Does NOT render a status bar
//   - Does NOT restore VTerm on exit (the caller should re-render)
//   - Clears the screen on entry (simple ESC[2J)
//
// The readerLoop continues running during passthrough — when
// passthroughActive is true, output chunks are also forwarded to
// passthroughOutput (stdout) in addition to the outputCh channel.
// This avoids a data race because only the BufferedReader reads from
// the PTY fd; the passthrough only reads from os.Stdin.
func (cs *CaptureSession) Passthrough(ctx context.Context, cfg PassthroughConfig) (ExitReason, error) {
	cs.mu.Lock()
	closed := cs.closed
	proc := cs.proc
	reader := cs.reader
	cs.mu.Unlock()
	if closed || proc == nil || reader == nil {
		return ExitError, ErrNoChild
	}

	// Save terminal state and enter raw mode.
	if cfg.TermFd >= 0 && cfg.TermState != nil {
		savedState, err := cfg.TermState.MakeRaw(cfg.TermFd)
		if err != nil {
			return ExitError, err
		}
		defer func() {
			_ = cfg.TermState.Restore(cfg.TermFd, savedState)
		}()

		// Ensure stdin fd is in blocking mode.
		if cfg.BlockingGuard != nil {
			origFlags, flagErr := cfg.BlockingGuard.EnsureBlocking(cfg.TermFd)
			if flagErr == nil {
				defer cfg.BlockingGuard.Restore(cfg.TermFd, origFlags)
			}
		}
	}

	// Clear screen and resize child to full terminal dimensions.
	if cfg.Stdout != nil {
		writeOrLog(cfg.Stdout, []byte("\x1b[2J\x1b[H"), "capture-passthrough-clear")
	}
	if cfg.TermFd >= 0 && cfg.TermState != nil {
		if w, h, err := cfg.TermState.GetSize(cfg.TermFd); err == nil {
			_ = proc.Resize(uint16(h), uint16(w))
		}
	}

	// Activate passthrough: readerLoop will forward output to stdout.
	cs.mu.Lock()
	cs.passthroughOutput = cfg.Stdout
	cs.passthroughActive = true
	cs.mu.Unlock()
	defer func() {
		cs.mu.Lock()
		cs.passthroughActive = false
		cs.passthroughOutput = nil
		cs.mu.Unlock()
	}()

	fwdCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	type fwdResult struct {
		reason ExitReason
		err    error
	}
	resultCh := make(chan fwdResult, 1)

	// Monitor for child exit: when the readerLoop's channel closes,
	// the child has exited.
	go func() {
		select {
		case <-fwdCtx.Done():
			return
		case <-cs.done:
			resultCh <- fwdResult{ExitChildExit, nil}
		}
	}()

	// Goroutine: stdin → session (input forwarding with toggle key detection).
	go func() {
		buf := make([]byte, 4096)
		for {
			select {
			case <-fwdCtx.Done():
				return
			default:
			}
			n, err := cfg.Stdin.Read(buf)
			if err != nil {
				if fwdCtx.Err() != nil {
					return
				}
				if errors.Is(err, syscall.EAGAIN) || errors.Is(err, syscall.EWOULDBLOCK) {
					runtime.Gosched()
					continue
				}
				resultCh <- fwdResult{ExitError, err}
				return
			}
			data := buf[:n]
			// Scan for toggle key.
			for i := 0; i < len(data); i++ {
				if data[i] == cfg.ToggleKey {
					if i > 0 {
						if err := proc.Write(string(data[:i])); err != nil {
							_ = err // best-effort before toggle exit
						}
					}
					resultCh <- fwdResult{ExitToggle, nil}
					return
				}
			}
			if err := proc.Write(string(data)); err != nil {
				if fwdCtx.Err() != nil {
					return
				}
				resultCh <- fwdResult{ExitError, err}
				return
			}
		}
	}()

	// Wait for any goroutine to signal completion.
	select {
	case r := <-resultCh:
		cancel()
		return r.reason, r.err
	case <-ctx.Done():
		cancel()
		return ExitContext, ctx.Err()
	}
}

// ensure CaptureSession implements io.Closer at compile time.
var _ io.Closer = (*CaptureSession)(nil)
var _ InteractiveSession = (*CaptureSession)(nil)
