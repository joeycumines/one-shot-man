// Package pty provides Go-level APIs for spawning and managing processes in
// pseudo-terminals. It uses creack/pty on Unix systems and ConPTY on Windows.
package pty

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"syscall"
	"time"
)

// SpawnConfig configures a PTY session.
type SpawnConfig struct {
	// Command is the executable path or name.
	Command string
	// Args are the command arguments.
	Args []string
	// Env contains additional environment variables merged with os.Environ().
	Env map[string]string
	// Dir is the working directory (default: caller's CWD).
	Dir string
	// Rows is the terminal row count (default: 24).
	Rows uint16
	// Cols is the terminal column count (default: 80).
	Cols uint16
	// Term is the TERM environment variable (default: "xterm-256color").
	Term string
	// WriteTimeout is the maximum duration for a single PTY write.
	// If zero, DefaultWriteTimeout is used. Set to a negative value to disable.
	WriteTimeout time.Duration
	// CloseGracePeriod is the duration Close waits after SIGTERM before
	// escalating to SIGKILL. If zero, DefaultCloseGracePeriod is used.
	CloseGracePeriod time.Duration
	// CloseForceWait is the duration Close waits after SIGKILL before
	// giving up and returning ErrForceKillTimeout. If zero,
	// DefaultCloseForceWait is used.
	CloseForceWait time.Duration
}

// Process represents a running process attached to a pseudo-terminal.
// All methods are safe for concurrent use unless otherwise noted.
type Process struct {
	mu                sync.Mutex
	ptyFile           *os.File // Platform-specific: PTY master (Unix) or ConPTY output pipe (Windows).
	writeFile         *os.File // Separate write pipe for ConPTY (Windows); nil on Unix where ptyFile is bidirectional.
	ttyFile           *os.File // Slave PTY fd, kept alive to prevent macOS EIO data loss. May be nil.
	closed            bool
	draining          bool
	done              chan struct{}
	exitCode          int
	exitErr           error
	cmd               processHandle // Platform-specific process handle.
	writeTimeout      time.Duration // From SpawnConfig; <=0 means no deadline.
	closeGracefulWait time.Duration // From SpawnConfig; duration to wait after SIGTERM.
	closeForceWait    time.Duration // From SpawnConfig; duration to wait after SIGKILL.
}

// processHandle is implemented per-platform (pty_unix.go, pty_windows.go).
// It wraps the os/exec.Cmd or equivalent for process lifecycle management.
type processHandle interface {
	Wait() error
	Signal(sig os.Signal) error
	Pid() int
}

// ErrNotSupported is returned on platforms where PTY is not available.
var ErrNotSupported = errors.New("pty: not supported on this platform")

// ErrClosed is returned when operating on a closed Process.
var ErrClosed = errors.New("pty: process is closed")

// ErrForceKillTimeout is returned by Close when the process does not exit
// within the configured CloseForceWait duration after receiving SIGKILL.
type ErrForceKillTimeout struct {
	Wait time.Duration
}

func (e *ErrForceKillTimeout) Error() string {
	return fmt.Sprintf("pty: force-kill wait timed out after %s", e.Wait)
}

func (e *ErrForceKillTimeout) Unwrap() error { return nil }

// Defaults for SpawnConfig.
const (
	DefaultRows             uint16        = 24
	DefaultCols             uint16        = 80
	DefaultTerm             string        = "xterm-256color"
	DefaultWriteTimeout     time.Duration = 30 * time.Second
	DefaultCloseGracePeriod time.Duration = 5 * time.Second
	DefaultCloseForceWait   time.Duration = 2 * time.Second
	DefaultReadBufferSize                 = 4096
)

// applyDefaults fills in zero values with defaults.
func (c *SpawnConfig) applyDefaults() {
	if c.Rows == 0 {
		c.Rows = DefaultRows
	}
	if c.Cols == 0 {
		c.Cols = DefaultCols
	}
	if c.Term == "" {
		c.Term = DefaultTerm
	}
	if c.WriteTimeout == 0 {
		c.WriteTimeout = DefaultWriteTimeout
	}
	if c.CloseGracePeriod == 0 {
		c.CloseGracePeriod = DefaultCloseGracePeriod
	}
	if c.CloseForceWait == 0 {
		c.CloseForceWait = DefaultCloseForceWait
	}
}

// ErrWriteTimeout is returned when a write exceeds the configured WriteTimeout
// and SetWriteDeadline is not supported for the underlying file descriptor
// (e.g., PTY master fds on macOS where os.File deadline support is limited
// to network connections and pipes).
var ErrWriteTimeout = errors.New("pty: write timed out")

// Write sends data to the PTY (the child process's stdin).
// The lock is released before the kernel write to avoid deadlocking
// with Signal, Close, or Resize (which also acquire the lock).
//
// If the Process was spawned with a WriteTimeout > 0, a write deadline
// is set before each write so a hung child cannot block the caller
// indefinitely. On platforms where SetWriteDeadline does not work for
// PTY fds (macOS), a goroutine-based timeout is used as a fallback.
// EAGAIN is handled transparently by Go's runtime poller; the JS layer
// also retries EAGAIN for defence-in-depth.
func (p *Process) Write(data []byte) (int, error) {
	// Don't hold lock during blocking write — just check closed state.
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return 0, ErrClosed
	}
	f := p.ptyFile
	if p.writeFile != nil {
		f = p.writeFile
	}
	wt := p.writeTimeout
	p.mu.Unlock()

	if wt > 0 {
		if err := f.SetWriteDeadline(time.Now().Add(wt)); err != nil {
			// SetWriteDeadline is not supported for this fd (e.g., PTY on macOS).
			// Fall back to goroutine-based timeout to prevent indefinite blocking.
			return p.writeWithGoroutineTimeout(f, data, wt)
		}
		defer func() { _ = f.SetWriteDeadline(time.Time{}) }() // Clear deadline.
	}

	return f.Write(data)
}

// writeWithGoroutineTimeout runs the write in a background goroutine and
// enforces the timeout with a timer. If the timeout fires before the write
// completes, ErrWriteTimeout is returned. The background goroutine may
// continue to block in the kernel; it will be unblocked when the PTY fd
// is closed (via Process.Close) or when the child process exits.
//
// This is the fallback path for platforms where SetWriteDeadline does not
// work on PTY file descriptors (observed on macOS where PTY master fds
// are not registered with the kqueue-based runtime poller).
func (p *Process) writeWithGoroutineTimeout(f *os.File, data []byte, timeout time.Duration) (int, error) {
	type writeResult struct {
		n   int
		err error
	}
	ch := make(chan writeResult, 1)
	// Clone the data slice before the background write. If the timeout
	// fires and the caller reuses the original buffer, the goroutine
	// must hold its own copy to avoid a data race with the kernel write.
	bufCopy := make([]byte, len(data))
	copy(bufCopy, data)
	go func() {
		n, err := f.Write(bufCopy)
		ch <- writeResult{n: n, err: err}
	}()
	select {
	case res := <-ch:
		return res.n, res.err
	case <-time.After(timeout):
		return 0, fmt.Errorf("%w: write did not complete within %s (SetWriteDeadline not supported on this fd)", ErrWriteTimeout, timeout)
	}
}

// Read reads available output from the PTY (the child process's stdout).
// DefaultReadBufferSize is the read buffer size for PTY read operations.
// Returns (nil, io.EOF) when the PTY is closed or the process exits.
func (p *Process) Read() ([]byte, error) {
	// Don't hold lock during blocking read — just check closed state.
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil, ErrClosed
	}
	f := p.ptyFile
	p.mu.Unlock()

	buf := make([]byte, DefaultReadBufferSize)
	n, err := f.Read(buf)
	if n > 0 {
		// Return a right-sized copy to avoid retaining the full DefaultReadBufferSize-byte
		// backing array when the caller holds onto the result.
		cp := make([]byte, n)
		copy(cp, buf[:n])
		return cp, err
	}
	return nil, err
}

// DrainOutput starts a background goroutine that continuously reads and
// discards all output from the PTY master (the child process's stdout).
// This prevents output buffer deadlocks when the caller communicates with
// the child via other channels (e.g., MCP callbacks) and does not read
// the PTY output directly.
//
// Without draining, a child process that writes to stdout (e.g., a TUI
// banner) can fill the kernel's PTY output buffer. Once full, the child
// blocks on stdout writes and stops reading from stdin, causing writes
// from the parent (via Write) to also block — a classic PTY deadlock.
//
// If sink is non-nil, drained output is written to it (for diagnostics).
// Otherwise output is discarded.
//
// The goroutine exits when Read returns an error (typically io.EOF when
// the PTY is closed or the process exits). DrainOutput is idempotent;
// subsequent calls after the first are no-ops.
//
// WARNING: Once draining is active, Read and Receive calls from other
// goroutines will compete with the drain goroutine for output data.
// If you need to display the child's output (e.g., via tuiMux), do NOT
// call DrainOutput — instead set up your own reader.
func (p *Process) DrainOutput(sink io.Writer) {
	p.mu.Lock()
	if p.draining || p.closed {
		p.mu.Unlock()
		return
	}
	p.draining = true
	f := p.ptyFile
	p.mu.Unlock()

	if sink == nil {
		sink = io.Discard
	}

	go func() {
		buf := make([]byte, DefaultReadBufferSize)
		for {
			n, err := f.Read(buf)
			if n > 0 {
				_, _ = sink.Write(buf[:n])
			}
			if err != nil {
				return
			}
		}
	}()
}

// Resize changes the PTY window dimensions, sending SIGWINCH to the child.
func (p *Process) Resize(rows, cols uint16) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return ErrClosed
	}
	return p.platformResize(rows, cols)
}

// Signal sends a signal to the child process.
// Supported signal names: "SIGINT", "SIGTERM", "SIGKILL", "SIGHUP", "SIGQUIT".
// On Unix: "SIGSTOP", "SIGCONT" are also supported (via extraSignals).
func (p *Process) Signal(sig string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return ErrClosed
	}
	osSig, err := parseSignal(sig)
	if err != nil {
		return err
	}
	return p.cmd.Signal(osSig)
}

// Wait blocks until the child process exits. Returns the exit code (0 for success).
// It is safe to call Wait concurrently; all callers receive the same result.
func (p *Process) Wait() (int, error) {
	<-p.done
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.exitCode, p.exitErr
}

// IsAlive returns true if the child process is still running.
func (p *Process) IsAlive() bool {
	select {
	case <-p.done:
		return false
	default:
		return true
	}
}

// Pid returns the child process PID. Returns 0 if not started.
func (p *Process) Pid() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cmd == nil {
		return 0
	}
	return p.cmd.Pid()
}

// File returns the PTY master file descriptor. The caller must NOT close
// the returned file — it is owned by the Process and will be closed by
// Close. Returns nil if the process has not been started or has been closed.
func (p *Process) File() *os.File {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || p.ptyFile == nil {
		return nil
	}
	return p.ptyFile
}

// ClosePseudoConsole closes the ConPTY pseudoconsole handle (Windows only).
// On Windows, this flushes the ConPTY's internal buffer to the output pipe
// and closes the pipe's write end, causing Read() to return remaining data
// followed by EOF. On Unix this is a no-op.
// Safe to call after the process has exited — use to unblock ReadLoop
// when the ConPTY output pipe doesn't close automatically on exit.
func (p *Process) ClosePseudoConsole() {
	p.platformClosePseudoConsole()
}

// Close terminates the child process and releases the PTY.
// It sends SIGTERM, waits up to CloseGracePeriod, then sends SIGKILL.
// If the process survives SIGKILL for CloseForceWait, ErrForceKillTimeout is returned.
// Close is idempotent — subsequent calls return nil.
func (p *Process) Close() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	cmd := p.cmd
	f := p.ptyFile
	tty := p.ttyFile
	wf := p.writeFile
	graceWait := p.closeGracefulWait
	forceWait := p.closeForceWait
	p.ttyFile = nil   // prevent double-close with Wait goroutine
	p.writeFile = nil // prevent double-close
	p.mu.Unlock()

	// Close the slave PTY BEFORE the grace/force kill loop.
	//
	// On macOS, the child runs as a session leader (Setsid+Setctty). A
	// session-leader process that has called exit() cannot complete its
	// transition to a reapable zombie state while any slave tty fd remains
	// open — it lingers in the macOS-specific "Exiting" (E) state, blocking
	// wait4 indefinitely. Spawn intentionally keeps the parent's slave fd
	// open during normal operation to prevent EIO data loss on the master
	// for fast-exiting commands; closing it here (during shutdown) releases
	// that hold so the wait goroutine can reap the child.
	//
	// This is safe: the child retains its own slave fds (dup'd at Start), so
	// closing the parent's copy does not deliver EIO while the child is alive,
	// and SIGTERM/SIGKILL still reach the child normally. Closing the slave
	// first means a child that has already logically exited (but is stuck in
	// E state) becomes reapable immediately, so Close returns promptly instead
	// of burning the full grace+force budget.
	//
	// Note: only the slave tty (ttyFile, Unix-only) is moved here. The write
	// pipe (writeFile, Windows ConPTY input) stays after the grace loop —
	// moving it would alter Windows close ordering without test coverage on
	// this platform. On Unix writeFile is always nil, so this is a no-op
	// for Windows.
	if tty != nil {
		_ = tty.Close()
	}

	// Try graceful shutdown first.
	forceKillTimedOut := false
	if p.IsAlive() {
		_ = cmd.Signal(syscall.SIGTERM)

		select {
		case <-p.done:
			// Process exited gracefully.
		case <-time.After(graceWait):
			// Force kill.
			_ = cmd.Signal(syscall.SIGKILL)
			select {
			case <-p.done:
			case <-time.After(forceWait):
				// Do not block forever — the process may be wedged
				// in an unreapable state on some platforms.
				forceKillTimedOut = true
			}
		}
	}

	// Platform-specific cleanup (e.g., ConPTY handle on Windows).
	p.platformClose()

	// Close write pipe (ConPTY input on Windows), then master.
	if wf != nil {
		_ = wf.Close()
	}
	closeErr := f.Close()
	if forceKillTimedOut {
		timeoutErr := &ErrForceKillTimeout{Wait: forceWait}
		if closeErr != nil {
			return fmt.Errorf("%w: %v", timeoutErr, closeErr)
		}
		return timeoutErr
	}
	return closeErr
}

// parseSignal converts a signal name string to an os.Signal.
// Platform-specific signals (SIGSTOP, SIGCONT) are resolved via
// extraSignals defined in pty_signal_{unix,windows}.go.
func parseSignal(name string) (os.Signal, error) {
	switch name {
	case "SIGINT":
		return syscall.SIGINT, nil
	case "SIGTERM":
		return syscall.SIGTERM, nil
	case "SIGKILL":
		return syscall.SIGKILL, nil
	case "SIGHUP":
		return syscall.SIGHUP, nil
	case "SIGQUIT":
		return syscall.SIGQUIT, nil
	default:
		if sig, ok := extraSignals[name]; ok {
			return sig, nil
		}
		return nil, errors.New("pty: unsupported signal: " + name)
	}
}

// ensure Process implements io.Writer at compile time.
var _ io.Writer = (*Process)(nil)
