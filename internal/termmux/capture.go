package termmux

import (
	"context"
	"errors"
	"io"
	"sync"
	"time"

	"github.com/joeycumines/one-shot-man/internal/termmux/pty"
	"github.com/joeycumines/one-shot-man/internal/termmux/vt"
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
}

// CaptureSession manages a PTY-attached command with real-time output capture
// via an in-memory VT100 emulator. It is a simplified, standalone alternative
// to Mux for cases where only output capture is needed — no terminal
// multiplexing, toggle keys, status bar, or raw-mode management.
//
// Usage:
//
//	cs := termmux.NewCaptureSession(termmux.CaptureConfig{
//	    Command: "make",
//	    Args:    []string{"test"},
//	    Dir:     "/path/to/project",
//	})
//	if err := cs.Start(ctx); err != nil { ... }
//	// Poll cs.Output() or cs.Screen() for progress.
//	exitCode, err := cs.Wait()
//	cs.Close()
//
// All methods are safe for concurrent use.
type CaptureSession struct {
	mu     sync.Mutex
	cfg    CaptureConfig
	proc   *pty.Process
	term   *vt.VTerm
	target SessionTarget

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
	kind := cfg.Kind
	if kind == SessionKindUnknown {
		kind = SessionKindCapture
	}
	return &CaptureSession{
		cfg:    cfg,
		term:   vt.NewVTerm(rows, cols),
		done:   make(chan struct{}),
		rows:   rows,
		cols:   cols,
		target: SessionTarget{Name: cfg.Name, Kind: kind},
	}
}

// Target returns the session identity metadata.
func (cs *CaptureSession) Target() SessionTarget {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if cs.target.Kind == SessionKindUnknown {
		return SessionTarget{Name: cs.cfg.Name, Kind: SessionKindCapture}
	}
	return cs.target
}

// SetTarget updates the session identity metadata.
func (cs *CaptureSession) SetTarget(target SessionTarget) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if target.Kind == SessionKindUnknown {
		target.Kind = SessionKindCapture
	}
	cs.target = target
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

	// Start the background reader that feeds PTY output to the VTerm.
	// The reader goroutine also captures exit status before signaling
	// completion, ensuring Wait() always returns the correct exit code.
	go cs.readerLoop()

	return nil
}

// readerLoop continuously reads from the PTY and writes to the VTerm.
// After the read loop exits (on EOF/error), it captures the process exit
// status and only then closes cs.done. This ordering guarantees that
// Wait() always returns the correct exit code — there is no race between
// a separate waitLoop writing exitCode and the done channel being closed.
func (cs *CaptureSession) readerLoop() {
	defer close(cs.done)

	// Drain all output from the PTY into the VTerm.
	for {
		data, err := cs.proc.Read()
		if len(data) > 0 {
			// VTerm.Write is internally synchronized (has its own mutex).
			_, _ = cs.term.Write([]byte(data))
		}
		if err != nil {
			break
		}
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

// IsRunning returns true if the child process has not yet exited.
func (cs *CaptureSession) IsRunning() bool {
	cs.mu.Lock()
	proc := cs.proc
	cs.mu.Unlock()
	if proc == nil {
		return false
	}
	return proc.IsAlive()
}

// Output returns a plain-text snapshot of the virtual terminal screen.
// Each row is a string of non-NUL characters (trailing spaces stripped),
// joined by newlines. Returns an empty string if the session has not been
// started. Thread-safe.
func (cs *CaptureSession) Output() string {
	return cs.term.String()
}

// Screen returns a full ANSI-escaped representation of the virtual terminal
// suitable for rendering in a terminal emulator. Returns an empty string if
// the session has not been started. Thread-safe.
func (cs *CaptureSession) Screen() string {
	cs.mu.Lock()
	started := cs.started
	cs.mu.Unlock()
	if !started {
		return ""
	}
	return cs.term.RenderFullScreen()
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

// Resize changes the PTY and VTerm dimensions. Returns an error if the
// session has not been started.
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
	// Resize PTY first (delivers SIGWINCH to child).
	if err := proc.Resize(uint16(rows), uint16(cols)); err != nil {
		return err
	}
	// Then resize VTerm so screen buffer matches.
	cs.term.Resize(rows, cols)
	cs.mu.Lock()
	cs.rows = rows
	cs.cols = cols
	cs.mu.Unlock()
	return nil
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
	proc := cs.proc
	cs.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if proc != nil {
		err := proc.Close()
		// Wait for reader loop to finish so all output is captured.
		// proc.Close() closes the PTY fd, which unblocks any pending
		// Read() call in readerLoop. The timeout is a safety net for
		// edge cases where fd closure doesn't unblock immediately.
		select {
		case <-cs.done:
		case <-time.After(5 * time.Second):
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

// ensure CaptureSession implements io.Closer at compile time.
var _ io.Closer = (*CaptureSession)(nil)
var _ InteractiveSession = (*CaptureSession)(nil)
