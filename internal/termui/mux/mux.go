// Package mux provides terminal multiplexing between osm's TUI and a child
// PTY process (e.g., Claude Code). The mux operates in two modes:
//
//   - osm mode: Normal operation. go-prompt reads from stdin.
//   - Claude mode: Raw byte forwarding between stdin/stdout and the child PTY.
//
// The mux uses a command-blocking model: when the user invokes the "claude"
// TUI command, [TUIMux.RunPassthrough] blocks until the user presses the
// toggle key (default Ctrl+]) or the child process exits. go-prompt is
// naturally paused during this time since its command handler has not returned.
//
// A single-line status bar on the bottom terminal row shows the active mode,
// Claude's status, and the toggle key hint. The status bar uses ANSI scroll
// region management to avoid interfering with either TUI's output.
package mux

import (
	"context"
	"errors"
	"fmt"
	"io"
	"runtime"
	"sync"
	"syscall"
	"time"

	"golang.org/x/term"
)

// Side represents which process owns the terminal.
type Side int

const (
	// SideOsm indicates osm's TUI owns the terminal.
	SideOsm Side = iota
	// SideClaude indicates the child PTY owns the terminal.
	SideClaude
)

// ExitReason describes why RunPassthrough returned.
type ExitReason int

const (
	// ExitToggle means the user pressed the toggle key.
	ExitToggle ExitReason = iota
	// ExitChildExit means the child process exited (EOF on PTY read).
	ExitChildExit
	// ExitContext means the context was cancelled.
	ExitContext
	// ExitError means an I/O error occurred.
	ExitError
)

// String returns a human-readable exit reason name.
func (r ExitReason) String() string {
	switch r {
	case ExitToggle:
		return "toggle"
	case ExitChildExit:
		return "child-exit"
	case ExitContext:
		return "context"
	case ExitError:
		return "error"
	default:
		return "unknown"
	}
}

// DefaultToggleKey is Ctrl+] (ASCII GS, 0x1D).
const DefaultToggleKey byte = 0x1D

var (
	// ErrNoChild is returned when RunPassthrough is called without an attached child.
	ErrNoChild = errors.New("mux: no child process attached")
	// ErrAlreadyAttached is returned when Attach is called while a child is attached.
	ErrAlreadyAttached = errors.New("mux: child already attached")
	// ErrPassthroughActive is returned when an operation conflicts with active passthrough.
	ErrPassthroughActive = errors.New("mux: passthrough is active")
)

// TUIMux manages terminal ownership between osm and a child PTY process.
// It is safe for concurrent use.
type TUIMux struct {
	mu sync.Mutex

	// Terminal I/O — set at construction, never changed.
	stdin  io.Reader
	stdout io.Writer
	termFd int // file descriptor for terminal state operations (-1 = no terminal)

	// Child process state — guarded by mu.
	child  io.ReadWriteCloser
	active Side

	// Configuration — guarded by mu.
	toggleKey byte

	// Resize callback — called when SIGWINCH propagation is needed.
	// If nil, resizes are not propagated to the child.
	resizeFn func(rows, cols uint16) error

	// Status bar configuration.
	statusEnabled bool
	claudeStatus  string // "idle", "thinking", "tool-use", "error"

	// Passthrough state — guarded by mu.
	passthroughActive bool

	// First-swap flag: on the first transition to Claude mode, the
	// terminal is cleared and a SIGWINCH is sent to the child so
	// Claude's TUI renders on a clean canvas. guarded by mu.
	swappedOnce bool

	// childVterm captures the child PTY's output in an in-memory
	// VT100 virtual terminal buffer. When the user toggles back to
	// Claude mode, the buffer is re-rendered to restore Claude's
	// screen state. Created on Attach, nilled on Detach.
	childVterm *VTerm

	// termRows and termCols track the last known terminal dimensions
	// for VTerm sizing. Updated on Attach and in RunPassthrough.
	termRows int
	termCols int

	// Background reader goroutine lifecycle.
	// bgReaderDone is closed when the background reader goroutine exits.
	// bgChildEOF is closed when the child sends EOF (process exited).
	bgReaderDone chan struct{}
	bgChildEOF   chan struct{}
}

// New creates a TUIMux. stdin and stdout are the real terminal streams.
// termFd is the file descriptor for terminal state operations (typically
// int(os.Stdin.Fd())). Pass -1 if no terminal is available (e.g., tests).
func New(stdin io.Reader, stdout io.Writer, termFd int) *TUIMux {
	return &TUIMux{
		stdin:         stdin,
		stdout:        stdout,
		termFd:        termFd,
		toggleKey:     DefaultToggleKey,
		active:        SideOsm,
		statusEnabled: true,
		claudeStatus:  "idle",
	}
}

// Attach registers a child PTY process. The child must implement
// io.ReadWriteCloser (the PTY's I/O interface). Returns ErrAlreadyAttached
// if a child is already attached.
func (m *TUIMux) Attach(child io.ReadWriteCloser) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.child != nil {
		return ErrAlreadyAttached
	}
	m.child = child
	// Create VTerm to capture child output for toggle-back restoration.
	// Default to 24x80 if terminal dimensions aren't known yet;
	// RunPassthrough will resize when it reads the actual terminal size.
	rows, cols := m.termRows, m.termCols
	if rows <= 0 {
		rows = 24
	}
	if cols <= 0 {
		cols = 80
	}
	m.childVterm = NewVTerm(rows, cols)

	// Start background reader goroutine. This runs for the lifetime of
	// the attachment, continuously reading from the child PTY and teeing
	// output to the VTerm buffer. When passthrough is active, it also
	// forwards to stdout. This prevents child process starvation (pipe
	// buffer full → child blocks) when in osm mode, and avoids spawning
	// a new read goroutine on every toggle cycle.
	m.bgReaderDone = make(chan struct{})
	m.bgChildEOF = make(chan struct{})
	go m.backgroundReader(child, m.bgReaderDone, m.bgChildEOF)

	return nil
}

// Detach disconnects the child PTY. Returns ErrPassthroughActive if
// passthrough is currently running.
//
// For immediate cleanup, close the child PTY before calling Detach so
// the background reader goroutine receives EOF and exits promptly. If
// the child is not closed before Detach, the method waits up to 3
// seconds for the reader to exit, then returns anyway — the reader
// will eventually exit when the child is closed later.
func (m *TUIMux) Detach() error {
	m.mu.Lock()
	if m.passthroughActive {
		m.mu.Unlock()
		return ErrPassthroughActive
	}
	bgDone := m.bgReaderDone
	m.child = nil
	m.childVterm = nil
	m.active = SideOsm
	m.swappedOnce = false // Reset so next Attach gets fresh first-swap behavior.
	m.bgReaderDone = nil
	m.bgChildEOF = nil
	m.mu.Unlock()

	// Best-effort wait for the background reader goroutine to finish.
	// The reader exits when child.Read() returns an error (EOF after
	// Close, or pipe broken). If the caller closed the child before
	// calling Detach, this returns immediately. Otherwise, we time out
	// after 3 seconds to avoid blocking indefinitely when the caller
	// closes the child after Detach (e.g. cleanupExecutor pattern).
	if bgDone != nil {
		select {
		case <-bgDone:
		case <-time.After(3 * time.Second):
		}
	}
	return nil
}

// backgroundReader is the persistent goroutine that reads from the child
// PTY for the lifetime of the attachment. It tees output to the VTerm
// buffer always, and forwards to stdout only when passthrough is active.
//
// This design prevents:
//   - Goroutine leaks: one goroutine per Attach, not per toggle cycle
//   - Child starvation: output is consumed even when in osm mode
//   - Read contention: only one goroutine ever reads from the child fd
func (m *TUIMux) backgroundReader(child io.ReadWriteCloser, bgDone, bgEOF chan struct{}) {
	defer close(bgDone)
	buf := make([]byte, 4096)
	for {
		n, err := child.Read(buf)
		if n > 0 {
			m.mu.Lock()
			vt := m.childVterm
			active := m.passthroughActive
			m.mu.Unlock()

			// If Detach() was called (childVterm is nil), stop
			// processing output. The reader will exit on the next
			// Read error (when the child fd is eventually closed)
			// or we can bail out early here.
			if vt == nil {
				// Detached — drain remaining data but don't forward.
				if err != nil {
					close(bgEOF)
					return
				}
				continue
			}

			// Always tee to VTerm for screen restoration.
			_, _ = vt.Write(buf[:n])

			// Forward to stdout only during passthrough.
			// The lock is held across the write so that when
			// RunPassthrough sets passthroughActive=false under the
			// same lock, any in-flight write is guaranteed to have
			// completed before the caller reads from stdout.
			if active {
				m.mu.Lock()
				if m.passthroughActive {
					_, _ = m.stdout.Write(buf[:n])
				}
				m.mu.Unlock()
			}
		}
		if err != nil {
			close(bgEOF)
			return
		}
	}
}

// ActiveSide returns which side currently owns the terminal.
func (m *TUIMux) ActiveSide() Side {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.active
}

// SetToggleKey changes the key that exits passthrough mode.
// Default is Ctrl+] (0x1D).
func (m *TUIMux) SetToggleKey(key byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.toggleKey = key
}

// SetResizeFunc sets a callback invoked when the terminal is resized
// during passthrough. The callback should propagate dimensions to the
// child PTY (e.g., pty.Resize). Set to nil to disable propagation.
func (m *TUIMux) SetResizeFunc(fn func(rows, cols uint16) error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.resizeFn = fn
}

// SetStatusEnabled controls whether the status bar is rendered.
func (m *TUIMux) SetStatusEnabled(enabled bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.statusEnabled = enabled
}

// HasChild returns true if a child process is currently attached.
func (m *TUIMux) HasChild() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.child != nil
}

// SetClaudeStatus updates the Claude status shown in the status bar.
// Valid values: "idle", "thinking", "tool-use", "error".
func (m *TUIMux) SetClaudeStatus(status string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.claudeStatus = status
}

// WriteToChild sends data to the attached child PTY. Returns ErrNoChild
// if no child is attached. Safe for concurrent use.
func (m *TUIMux) WriteToChild(data []byte) (int, error) {
	m.mu.Lock()
	child := m.child
	m.mu.Unlock()
	if child == nil {
		return 0, ErrNoChild
	}
	return child.Write(data)
}

// RunPassthrough enters Claude mode: raw byte forwarding between
// stdin/stdout and the child PTY. This method blocks until:
//   - The user presses the toggle key (ExitToggle)
//   - The child process exits/EOF (ExitChildExit)
//   - The context is cancelled (ExitContext)
//   - An I/O error occurs (ExitError)
//
// Terminal state is saved before entering passthrough and restored after.
// If statusEnabled is true, a status bar is rendered on the last terminal row.
//
// Returns the exit reason and any associated error. For ExitToggle and
// ExitChildExit, the error is nil.
func (m *TUIMux) RunPassthrough(ctx context.Context) (ExitReason, error) {
	m.mu.Lock()
	if m.child == nil {
		m.mu.Unlock()
		return ExitError, ErrNoChild
	}
	if m.passthroughActive {
		m.mu.Unlock()
		return ExitError, ErrPassthroughActive
	}
	child := m.child
	toggleKey := m.toggleKey
	statusEnabled := m.statusEnabled
	m.passthroughActive = true
	m.active = SideClaude
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		m.passthroughActive = false
		m.active = SideOsm
		m.mu.Unlock()
	}()

	// Save and set terminal state.
	var savedState *term.State
	if m.termFd >= 0 {
		var err error
		savedState, err = term.MakeRaw(m.termFd)
		if err != nil {
			return ExitError, err
		}
		defer func() {
			_ = term.Restore(m.termFd, savedState)
		}()

		// Ensure stdin fd is in blocking mode. Libraries like go-prompt and
		// BubbleTea's cancelreader may leave the fd with O_NONBLOCK set.
		// Go's os.File.Read() does NOT handle EAGAIN for TTY fds — it
		// surfaces the error directly — so we must clear it here.
		origFlags, flagErr := ensureBlockingFd(m.termFd)
		if flagErr == nil {
			defer restoreBlockingFd(m.termFd, origFlags)
		}
		// If ensureBlockingFd fails, proceed anyway — the EAGAIN retry
		// loop in the stdin goroutine provides defense-in-depth.
	}

	// Get terminal dimensions for scroll region and VTerm sizing.
	var statusBarHeight int // non-zero when status bar is active
	// statusBarLines is 1 when status bar is rendered (reserving the
	// last terminal row), 0 otherwise. Subtracted from terminal height
	// for VTerm sizing and child PTY resize so the child's scroll
	// behavior matches the real terminal's constrained scroll region.
	var statusBarLines int
	if statusEnabled && m.termFd >= 0 {
		_, h, err := term.GetSize(m.termFd)
		if err == nil && h > 1 {
			statusBarHeight = h
			statusBarLines = 1
			m.mu.Lock()
			m.setScrollRegion(h)
			m.mu.Unlock()
			defer func() {
				m.mu.Lock()
				m.resetScrollRegion()
				m.mu.Unlock()
			}()
			m.mu.Lock()
			m.renderStatusBar(h)
			m.mu.Unlock()
		} else {
			statusEnabled = false
		}
	}
	// Update terminal dimensions and resize VTerm if needed.
	// When status bar is active, VTerm is sized to h - statusBarLines
	// so its scroll region matches the real terminal (rows 1..h-1).
	if m.termFd >= 0 {
		if w, h, err := term.GetSize(m.termFd); err == nil {
			childRows := h - statusBarLines
			m.mu.Lock()
			m.termRows = h
			m.termCols = w
			if m.childVterm != nil {
				m.childVterm.Resize(childRows, w)
			}
			m.mu.Unlock()
		}
	}

	// On the very first swap to Claude mode, clear the screen so
	// Claude's TUI renders from a clean state, and nudge the child
	// with a resize so it redraws at the correct dimensions.
	m.mu.Lock()
	firstSwap := !m.swappedOnce
	m.swappedOnce = true
	resizeFn := m.resizeFn
	m.mu.Unlock()
	if firstSwap {
		// ESC[2J = erase entire display, ESC[H = cursor to 1,1.
		// Hold m.mu during stdout writes to serialize with
		// backgroundReader's forwarding writes.
		m.mu.Lock()
		_, _ = m.stdout.Write([]byte("\x1b[2J\x1b[H"))
		m.mu.Unlock()
		if resizeFn != nil && m.termFd >= 0 {
			if w, h, err := term.GetSize(m.termFd); err == nil {
				// Tell child PTY about the usable rows (excluding
				// status bar). This ensures the child's layout
				// matches the scroll region on the real terminal.
				_ = resizeFn(uint16(h-statusBarLines), uint16(w))
			}
		}
	} else {
		// Not first swap — restore Claude's screen from VTerm buffer.
		// This re-renders the terminal content that was captured during
		// the previous passthrough session, so the user sees Claude's
		// output restored instead of a blank screen.
		m.mu.Lock()
		vt := m.childVterm
		m.mu.Unlock()
		if vt != nil {
			// Render outside m.mu (VTerm has its own lock), then
			// write to stdout under m.mu to avoid racing with
			// backgroundReader's forwarding writes.
			rendered := vt.Render()
			m.mu.Lock()
			_, _ = m.stdout.Write([]byte("\x1b[2J\x1b[H"))
			_, _ = m.stdout.Write([]byte(rendered))
			m.mu.Unlock()
		}
		// Re-render status bar after VTerm restore. The screen clear
		// above erases the status bar, and VTerm content doesn't
		// include status bar bytes. Re-paint it on the last row.
		if statusBarHeight > 0 {
			m.mu.Lock()
			m.renderStatusBar(statusBarHeight)
			m.mu.Unlock()
		}
	}

	// Capture bgChildEOF channel for detecting child exit.
	m.mu.Lock()
	bgChildEOF := m.bgChildEOF
	m.mu.Unlock()

	// Create cancellable context for goroutines.
	fwdCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Result channel — stdin goroutine sends the exit reason.
	type fwdResult struct {
		reason ExitReason
		err    error
	}
	resultCh := make(chan fwdResult, 1)

	// Goroutine: stdin → child PTY (with toggle key interception).
	// The background reader goroutine (started by Attach) handles
	// child → stdout forwarding, so we don't need a second goroutine here.
	go func() {
		buf := make([]byte, 4096)
		for {
			select {
			case <-fwdCtx.Done():
				return
			default:
			}
			n, err := m.stdin.Read(buf)
			if err != nil {
				if fwdCtx.Err() != nil {
					return
				}
				// Defense-in-depth: if stdin is in non-blocking mode
				// despite ensureBlockingFd, retry on EAGAIN instead of
				// surfacing the error and killing the passthrough.
				if errors.Is(err, syscall.EAGAIN) || errors.Is(err, syscall.EWOULDBLOCK) {
					runtime.Gosched()
					continue
				}
				resultCh <- fwdResult{ExitError, err}
				return
			}
			// Scan for toggle key.
			for i := 0; i < n; i++ {
				if buf[i] == toggleKey {
					// Forward bytes before the toggle key, then exit.
					if i > 0 {
						_, _ = child.Write(buf[:i])
					}
					resultCh <- fwdResult{ExitToggle, nil}
					return
				}
			}
			// Forward all bytes to child.
			if _, err := child.Write(buf[:n]); err != nil {
				if fwdCtx.Err() != nil {
					return
				}
				resultCh <- fwdResult{ExitError, err}
				return
			}
		}
	}()

	// Wait for first exit signal, child EOF, or context cancellation.
	select {
	case r := <-resultCh:
		cancel()
		return r.reason, r.err
	case <-bgChildEOF:
		cancel()
		return ExitChildExit, nil
	case <-ctx.Done():
		cancel()
		return ExitContext, ctx.Err()
	}
}

// setScrollRegion restricts scrolling to rows 1..(height-1), reserving
// the last row for the status bar.
func (m *TUIMux) setScrollRegion(height int) {
	_, _ = fmt.Fprintf(m.stdout, "\033[1;%dr\033[1;1H", height-1)
}

// resetScrollRegion restores the full terminal scroll region.
func (m *TUIMux) resetScrollRegion() {
	_, _ = m.stdout.Write([]byte("\033[r\033[999;1H"))
}

// renderStatusBar draws the status bar on the last terminal row.
func (m *TUIMux) renderStatusBar(height int) {
	m.mu.Lock()
	status := m.claudeStatus
	m.mu.Unlock()

	// Save cursor, move to last row, clear line, render, restore cursor.
	_, _ = fmt.Fprintf(m.stdout,
		"\033[s\033[%d;1H\033[2K\033[7m [Claude] %s │ Ctrl+] to switch \033[0m\033[u",
		height, status)
}

// StringIO is an interface compatible with string-based agent handles
// (e.g., claudemux.AgentHandle). Use [WrapStringIO] to adapt it
// to [io.ReadWriteCloser] for use with [TUIMux.Attach].
type StringIO interface {
	Send(input string) error
	Receive() (string, error)
	Close() error
}

// WrapStringIO adapts a [StringIO] (string-based I/O) to [io.ReadWriteCloser]
// (byte-based I/O). The adapter handles buffering for Receive→Read conversion.
func WrapStringIO(s StringIO) io.ReadWriteCloser {
	return &stringIOAdapter{inner: s}
}

type stringIOAdapter struct {
	inner StringIO
	buf   []byte
}

func (a *stringIOAdapter) Read(p []byte) (int, error) {
	// Drain buffered data first.
	if len(a.buf) > 0 {
		n := copy(p, a.buf)
		a.buf = a.buf[n:]
		return n, nil
	}
	// Read new data from the string-based source.
	s, err := a.inner.Receive()
	if len(s) > 0 {
		data := []byte(s)
		n := copy(p, data)
		if n < len(data) {
			a.buf = data[n:]
		}
		return n, err
	}
	return 0, err
}

func (a *stringIOAdapter) Write(p []byte) (int, error) {
	if err := a.inner.Send(string(p)); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (a *stringIOAdapter) Close() error {
	return a.inner.Close()
}
