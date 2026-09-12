package termmux

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"maps"
	"runtime"
	"sync"
	"time"

	"github.com/joeycumines/one-shot-man/internal/termmux/pty"
	"github.com/joeycumines/one-shot-man/internal/termmux/ptyio"
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
	// Rows is the virtual terminal row count (default: DefaultRows).
	Rows int
	// Cols is the virtual terminal column count (default: DefaultCols).
	Cols int
	// DrainTimeout is the maximum time Close() waits for the reader loop
	// to finish after the PTY is closed. Defaults to 5 seconds.
	// A negative value means "use default" (disables the override).
	DrainTimeout time.Duration
	// SkipDrain, when true, causes Close() to return immediately without
	// waiting for the reader loop to finish. This is useful when the caller
	// does not need to capture remaining output (e.g., after Kill()).
	SkipDrain bool
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
	doneOnce sync.Once
	cancel   context.CancelFunc
	exitCode int
	exitErr  error

	// Terminal dimensions (may change via Resize).
	rows int
	cols int

	// Buffered reader for PTY output. Set during Start; used by
	// readerLoop which consumes from reader.Output() channel.
	reader       *ptyio.BufferedReader
	readerCtx    context.Context
	readerCancel context.CancelFunc // cancels BufferedReader.ReadLoop

	// outputCh streams raw PTY output for consumption by SessionManager via
	// Reader(). An unbounded, synchronized queue sits in front of the public
	// channel so a slow or absent consumer cannot make the PTY reader drop data.
	outputCh           chan []byte
	outputMu           sync.Mutex
	outputQueue        [][]byte
	outputWake         chan struct{}
	outputClosed       bool
	outputDispatchDone chan struct{}

	// outputModeWake interrupts a public-channel send when passthrough changes
	// ownership. Mode 1 pauses dispatch so Passthrough can drain chunks already
	// delivered to outputCh; mode 2 routes all subsequent queued chunks directly
	// to passthroughOutput.
	outputModeWake    chan struct{}
	outputModeAck     chan struct{}
	outputModeAckSent bool

	// Passthrough state is consumed by outputLoop, which is the sole owner of
	// forwarding queued chunks. readerLoop only appends to outputQueue; this
	// prevents a chunk from being split between the public channel and stdout
	// during activation.
	passthroughActive  bool
	passthroughOutput  io.Writer  // set before activating passthrough
	passthroughWriteMu sync.Mutex // serializes activation flush and live forwarding

}

// NewCaptureSession creates a new capture session with the given configuration.
// The session is not started until Start is called.
func NewCaptureSession(cfg CaptureConfig) *CaptureSession {
	rows := cfg.Rows
	if rows <= 0 {
		rows = DefaultRows
	}
	cols := cfg.Cols
	if cols <= 0 {
		cols = DefaultCols
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

func (cs *CaptureSession) closeDone() {
	cs.doneOnce.Do(func() { close(cs.done) })
}

// Start spawns the command in a PTY and begins capturing output. The context
// controls the lifetime of the underlying process — cancelling it sends
// SIGKILL to the child. Start may be called only once; subsequent calls
// return an error.
func (cs *CaptureSession) Start(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
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
		// Start is a one-shot operation. Keep started set so the already-closed
		// completion channel cannot be reused by a later successful attempt.
		// Preserve the failed lifecycle result so Wait cannot report success.
		cs.exitCode = -1
		cs.exitErr = err
		cs.closeDone()
		cs.mu.Unlock()
		return err
	}

	// Finish installing the process and reader under one lock. Close may race
	// with Spawn; once proc is visible it must also be able to cancel the
	// reader before returning.
	readerCtx, readerCancel := context.WithCancel(childCtx)
	reader := ptyio.NewBufferedReader(proc.File(), 16)
	cs.mu.Lock()
	if cs.closed {
		// Close may have observed the pre-spawn state, leaving no reader loop
		// to close done. Roll back the started state and publish completion so
		// Wait and Done remain well-defined for this failed start.
		cs.closeDone()
		cs.mu.Unlock()
		readerCancel()
		cancel()
		_ = proc.Close()
		_, _ = proc.Wait()
		return errors.New("capture: session was closed while starting")
	}
	cs.proc = proc
	cs.cancel = cancel
	cs.reader = reader
	cs.readerCtx = readerCtx
	cs.readerCancel = readerCancel
	cs.outputCh = make(chan []byte, DefaultChannelBuffer)
	cs.outputWake = make(chan struct{}, 1)
	cs.outputDispatchDone = make(chan struct{})
	cs.outputModeWake = make(chan struct{}, 1)
	cs.outputModeAck = make(chan struct{})
	cs.outputModeAckSent = false
	cs.mu.Unlock()
	go reader.ReadLoop(readerCtx)
	go cs.outputLoop(readerCtx)

	// The buffered reader wraps the PTY descriptor so readerLoop can consume
	// output without racing passthrough access to the descriptor.
	// On Windows (ConPTY), the output pipe is not closed automatically when the
	// child exits, so ReadLoop would block on Read() forever. ClosePseudoConsole
	// flushes ConPTY's internal buffer, closes the pipe's write end, and lets
	// Read() return remaining data followed by EOF — no data is lost — which
	// unblocks ReadLoop and cascades to readerLoop closing outputCh and cs.done
	// for SessionManager and passthrough consumers. On Unix it is a no-op
	// (PTY closes naturally).

	if runtime.GOOS == "windows" {
		go func() {
			_, _ = proc.Wait()
			proc.ClosePseudoConsole()
		}()
	}

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
	defer cs.closeDone()

	cs.mu.Lock()
	reader := cs.reader
	cs.mu.Unlock()

	// Drain all output from the BufferedReader into the owned queue. Queueing
	// is deliberately independent of the public channel: a consumer that has
	// not attached yet must not apply backpressure to the PTY reader.
	for chunk := range reader.Output() {
		cp := make([]byte, len(chunk))
		copy(cp, chunk)
		cs.outputMu.Lock()
		cs.outputQueue = append(cs.outputQueue, cp)
		wake := cs.outputWake
		cs.outputMu.Unlock()
		select {
		case wake <- struct{}{}:
		default:
		}
	}

	// BufferedReader has reached EOF, so process exit can be collected now.
	code, err := cs.proc.Wait()
	cs.mu.Lock()
	cs.exitCode = code
	cs.exitErr = err
	cs.mu.Unlock()

	cs.outputMu.Lock()
	cs.outputClosed = true
	wake := cs.outputWake
	cs.outputMu.Unlock()
	select {
	case wake <- struct{}{}:
	default:
	}
	// Done tracks PTY/process completion. The dispatcher remains available for a
	// consumer that attaches after natural exit; Close cancels it explicitly when
	// a caller wants to abandon any still-queued output.
	cs.closeDone()
}

// outputLoop forwards queued output either to Reader or, after activation,
// directly to passthroughOutput. It owns that choice for every queued chunk,
// so activation cannot leave stale queue entries stranded on outputCh.
func (cs *CaptureSession) outputLoop(ctx context.Context) {
	defer close(cs.outputDispatchDone)
	defer close(cs.outputCh)

	for {
		cs.mu.Lock()
		active := cs.passthroughActive
		output := cs.passthroughOutput
		ackSent := cs.outputModeAckSent
		ack := cs.outputModeAck
		cs.mu.Unlock()

		if active {
			// Chunks already delivered to outputCh precede chunks still in the
			// owned queue. Drain those first so activation preserves order. A nil
			// output intentionally discards the public stream during passthrough.
			for {
				select {
				case chunk, ok := <-cs.outputCh:
					if !ok {
						goto publicDrained
					}
					if output != nil {
						cs.passthroughWriteMu.Lock()
						_ = writeOrLog(output, chunk, "capture-passthrough-output")
						cs.passthroughWriteMu.Unlock()
					}
				default:
					goto publicDrained
				}
			}
		publicDrained:
			if !ackSent {
				cs.mu.Lock()
				if cs.passthroughActive && !cs.outputModeAckSent {
					cs.outputModeAckSent = true
					close(ack)
				}
				cs.mu.Unlock()
			}
		}

		cs.outputMu.Lock()
		if len(cs.outputQueue) > 0 {
			chunk := cs.outputQueue[0]
			cs.outputQueue = cs.outputQueue[1:]
			cs.outputMu.Unlock()

			cs.mu.Lock()
			active = cs.passthroughActive
			output = cs.passthroughOutput
			cs.mu.Unlock()
			if active {
				if output != nil {
					cs.passthroughWriteMu.Lock()
					_ = writeOrLog(output, chunk, "capture-passthrough-output")
					cs.passthroughWriteMu.Unlock()
				}
				continue
			}

			select {
			case cs.outputCh <- chunk:
			case <-cs.outputModeWake:
				cs.outputMu.Lock()
				cs.outputQueue = append([][]byte{chunk}, cs.outputQueue...)
				cs.outputMu.Unlock()
			case <-ctx.Done():
				return
			}
			continue
		}
		closed := cs.outputClosed
		wake := cs.outputWake
		cs.outputMu.Unlock()
		if closed {
			return
		}
		select {
		case <-wake:
		case <-cs.outputModeWake:
		case <-ctx.Done():
			return
		}
	}
}

func (cs *CaptureSession) signalOutputMode() {
	select {
	case cs.outputModeWake <- struct{}{}:
	default:
	}
}

func (cs *CaptureSession) activatePassthrough(ctx context.Context, output io.Writer) error {
	cs.mu.Lock()
	cs.passthroughOutput = output
	cs.passthroughActive = true
	cs.outputModeAck = make(chan struct{})
	cs.outputModeAckSent = false
	ack := cs.outputModeAck
	cs.mu.Unlock()
	cs.signalOutputMode()
	select {
	case <-ack:
		return nil
	case <-ctx.Done():
		cs.mu.Lock()
		cs.passthroughActive = false
		cs.passthroughOutput = nil
		cs.mu.Unlock()
		cs.signalOutputMode()
		return ctx.Err()
	}
}

func (cs *CaptureSession) deactivatePassthrough() {
	cs.mu.Lock()
	cs.passthroughActive = false
	cs.passthroughOutput = nil
	cs.mu.Unlock()
	cs.signalOutputMode()
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
	// ConPTY on Windows does not support process suspension, regardless of
	// whether the session has been started or is already paused.
	if runtime.GOOS == "windows" {
		return ErrPauseNotSupported
	}
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
	// ConPTY on Windows does not support process resumption, regardless of
	// state.
	if runtime.GOOS == "windows" {
		return ErrResumeNotSupported
	}
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

// Wait blocks until the child process exits and the PTY reader has captured
// all available output. The returned Reader channel may still contain queued
// chunks when no consumer was attached; callers may drain it after Wait.
// Returns the exit code and any process error. Returns an error immediately
// if the session has not been started.
func (cs *CaptureSession) Wait() (int, error) {
	return cs.WaitContext(context.Background())
}

// WaitContext waits for completion or context cancellation. The caller can
// pair cancellation with Kill to interrupt the underlying process.
func (cs *CaptureSession) WaitContext(ctx context.Context) (int, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	cs.mu.Lock()
	started := cs.started
	cs.mu.Unlock()
	if !started {
		return -1, errors.New("capture: not started")
	}
	select {
	case <-cs.done:
		cs.mu.Lock()
		defer cs.mu.Unlock()
		return cs.exitCode, cs.exitErr
	case <-ctx.Done():
		return -1, ctx.Err()
	}
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
	started := cs.started
	if !started {
		cs.closeDone()
	}
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
		if !cs.cfg.SkipDrain {
			// Wait for reader loop to finish so all output is captured.
			// proc.Close() closes the PTY fd, which causes the BufferedReader's
			// ReadLoop to exit, which closes the output channel, which causes
			// readerLoop to exit. The timeout is a safety net for edge cases
			// where fd closure doesn't unblock immediately.
			select {
			case <-cs.done:
			case <-time.After(cs.cfg.DrainTimeout):
			}
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

// Rows returns the current terminal row count.
func (cs *CaptureSession) Rows() int {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return cs.rows
}

// Cols returns the current terminal column count.
func (cs *CaptureSession) Cols() int {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return cs.cols
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
		maps.Copy(cfg.Env, cs.cfg.Env)
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
	n, err := proc.Write(data)
	return n, err
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
			if err := cfg.TermState.Restore(cfg.TermFd, savedState); err != nil {
				slog.Debug("capture terminal restore failed", "error", err)
			}
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
		_ = writeOrLog(cfg.Stdout, []byte("\x1b[2J\x1b[H"), "capture-passthrough-clear")
	}
	if cfg.TermFd >= 0 && cfg.TermState != nil {
		if w, h, err := cfg.TermState.GetSize(cfg.TermFd); err == nil {
			_ = proc.Resize(uint16(h), uint16(w))
		}
	}

	// Activate passthrough through the output dispatcher. The dispatcher first
	// observes the mode change and drains its internal queue into stdout; only
	// then do we flush chunks already delivered to the public channel. This
	// creates one ordering barrier for both queue ownership and activation.
	fwdCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	if err := cs.activatePassthrough(fwdCtx, cfg.Stdout); err != nil {
		return ExitContext, err
	}
	var pending [][]byte
drainLoop:
	for {
		select {
		case b, ok := <-cs.outputCh:
			if !ok {
				break drainLoop
			}
			pending = append(pending, b)
		default:
			break drainLoop
		}
	}
	cs.passthroughWriteMu.Lock()
	for _, b := range pending {
		_ = writeOrLog(cfg.Stdout, b, "capture-passthrough-flush")
	}
	cs.passthroughWriteMu.Unlock()
	defer cs.deactivatePassthrough()

	// The output dispatcher owns all subsequent forwarding.

	resultCh := make(chan forwardResult, 1)

	// Monitor for child exit: when the readerLoop's channel closes,
	// the child has exited.
	go func() {
		select {
		case <-fwdCtx.Done():
			return
		case <-cs.done:
			resultCh <- forwardResult{ExitChildExit, nil}
		}
	}()

	// Stdin → PTY forwarding (shared with SessionManager.Passthrough).
	go forwardStdin(fwdCtx, resultCh, forwardConfig{
		Stdin:     cfg.Stdin,
		Writer:    proc,
		ToggleKey: cfg.ToggleKey,
	})

	// Signal forwarding (SIGINT, SIGQUIT, SIGTSTP).
	sigResultCh := make(chan signalResult, 1)
	sigCancel := watchSignals(ctx, sigResultCh, cfg.SignalChild)
	defer sigCancel()

	// Wait for any goroutine to signal completion. Context cancellation
	// takes priority: when the parent context is cancelled, the child
	// process is killed and cs.done closes, so resultCh may fire
	// simultaneously with ctx.Done(). Checking ctx.Err() after receiving
	// from resultCh ensures the correct exit reason in the race.
	select {
	case r := <-resultCh:
		cancel()
		sigCancel()
		if ctx.Err() != nil {
			return ExitContext, ctx.Err()
		}
		return r.reason, r.err
	case sr := <-sigResultCh:
		cancel()
		sigCancel()
		return sr.reason, sr.err
	case <-ctx.Done():
		cancel()
		sigCancel()
		return ExitContext, ctx.Err()
	}
}

// ensure CaptureSession implements io.Closer at compile time.
var _ io.Closer = (*CaptureSession)(nil)
var _ InteractiveSession = (*CaptureSession)(nil)
