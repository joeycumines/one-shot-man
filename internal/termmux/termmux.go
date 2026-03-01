package termmux

import (
	"context"
	"errors"
	"io"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/joeycumines/one-shot-man/internal/termmux/ptyio"
	"github.com/joeycumines/one-shot-man/internal/termmux/statusbar"
	"github.com/joeycumines/one-shot-man/internal/termmux/vt"
)

// Mux is a terminal multiplexer for toggling between an osm TUI and a
// Claude child process, with VTerm-based screen capture and restoration.
type Mux struct {
	mu sync.Mutex

	cfg    Config
	stdin  io.Reader
	stdout io.Writer
	termFd int

	// Child process state.
	child             io.ReadWriteCloser
	active            Side
	passthroughActive bool
	swappedOnce       bool

	// Screen capture.
	vterm *vt.VTerm

	// Terminal dimensions.
	termRows int
	termCols int

	// Background reader / tee goroutine.
	reader       *ptyio.BufferedReader
	readerCancel context.CancelFunc // cancels ReadLoop on Detach
	teeDone      chan struct{}
	childEOF     chan struct{}

	// Abstracted platform operations.
	statusBar     *statusbar.StatusBar
	termState     ptyio.TermState
	blockingGuard ptyio.BlockingGuard

	// Detach timeout.
	detachTimeout time.Duration
}

// New creates a new Mux with the given I/O and terminal fd.
func New(stdin io.Reader, stdout io.Writer, termFd int, opts ...Option) *Mux {
	cfg := defaultConfig()
	applyOptions(&cfg, opts)

	m := &Mux{
		cfg:           cfg,
		stdin:         stdin,
		stdout:        stdout,
		termFd:        termFd,
		active:        SideOsm,
		termRows:      24,
		termCols:      80,
		statusBar:     statusbar.New(stdout),
		termState:     ptyio.RealTermState{},
		blockingGuard: defaultBlockingGuard(),
		detachTimeout: 5 * time.Second,
	}
	m.statusBar.SetToggleKey(cfg.ToggleKey)
	if cfg.InitialStatus != "" {
		m.statusBar.SetStatus(cfg.InitialStatus)
	}
	return m
}

// Attach connects a child process to the mux for output capture and passthrough.
func (m *Mux) Attach(child io.ReadWriteCloser) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.child != nil {
		return ErrAlreadyAttached
	}
	m.child = child
	m.vterm = vt.NewVTerm(m.termRows, m.termCols)
	m.childEOF = make(chan struct{})
	m.teeDone = make(chan struct{})

	// Start buffered reader with a cancellable context so Detach()
	// can signal the ReadLoop to stop after its next successful read.
	m.reader = ptyio.NewBufferedReader(child, 16)
	readerCtx, readerCancel := context.WithCancel(context.Background())
	m.readerCancel = readerCancel
	go m.reader.ReadLoop(readerCtx)

	// Start tee goroutine. Capture reader, teeDone, and childEOF locally
	// so Detach can nil m.reader without racing, and so the goroutine
	// closes its own teeDone/childEOF channels (not any future ones
	// created by a subsequent Attach after a Detach timeout).
	reader := m.reader
	teeDone := m.teeDone
	childEOF := m.childEOF
	go m.teeLoop(reader, teeDone, childEOF)

	return nil
}

// HasChild returns true if a child process is attached.
func (m *Mux) HasChild() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.child != nil
}

func (m *Mux) teeLoop(reader *ptyio.BufferedReader, teeDone, childEOF chan struct{}) {
	defer close(teeDone)
	for chunk := range reader.Output() {
		m.mu.Lock()
		vtm := m.vterm
		passthrough := m.passthroughActive
		m.mu.Unlock()

		if vtm == nil {
			// Detached — stop processing.
			continue
		}

		// Always tee to VTerm for screen capture.
		_, _ = vtm.Write(chunk)

		// Forward to stdout only during passthrough.
		if passthrough {
			m.mu.Lock()
			if m.passthroughActive {
				_, _ = m.stdout.Write(chunk)
			}
			m.mu.Unlock()
		}
	}

	// Reader output channel closed → child EOF.
	select {
	case <-childEOF:
	default:
		close(childEOF)
	}
}

// Detach disconnects the child process from the mux.
func (m *Mux) Detach() error {
	m.mu.Lock()
	if m.passthroughActive {
		m.mu.Unlock()
		return ErrPassthroughActive
	}
	teeDone := m.teeDone
	readerCancel := m.readerCancel
	m.child = nil
	m.vterm = nil
	m.reader = nil
	m.readerCancel = nil
	m.active = SideOsm
	m.swappedOnce = false
	m.mu.Unlock()

	// Cancel the ReadLoop context so it exits after its next read.
	// This prevents goroutine accumulation when re-attaching the same
	// child handle (without this, each Attach creates a new ReadLoop
	// goroutine on the same fd, and old ones persist indefinitely).
	if readerCancel != nil {
		readerCancel()
	}

	if teeDone != nil {
		select {
		case <-teeDone:
		case <-time.After(m.detachTimeout):
		}
	}
	return nil
}

// ActiveSide returns which side currently owns the terminal.
func (m *Mux) ActiveSide() Side {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.active
}

// SetToggleKey changes the toggle key.
func (m *Mux) SetToggleKey(key byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cfg.ToggleKey = key
	m.statusBar.SetToggleKey(key)
}

// SetResizeFunc sets the callback for propagating resize to child PTY.
func (m *Mux) SetResizeFunc(fn func(rows, cols uint16) error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cfg.ResizeFn = fn
}

// SetStatusEnabled toggles the status bar.
func (m *Mux) SetStatusEnabled(enabled bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cfg.StatusEnabled = enabled
}

// SetClaudeStatus updates the status bar text.
func (m *Mux) SetClaudeStatus(status string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.statusBar.SetStatus(status)
}

// WriteToChild writes data to the child process.
func (m *Mux) WriteToChild(data []byte) (int, error) {
	m.mu.Lock()
	child := m.child
	m.mu.Unlock()
	if child == nil {
		return 0, ErrNoChild
	}
	return child.Write(data)
}

// ChildExitOutput returns the content captured in the VTerm buffer as
// plain text. This is useful for diagnostics when the child process
// exits unexpectedly — the buffer contains whatever the process wrote
// to stdout/stderr before dying (e.g., error messages, usage text).
// Returns an empty string if no VTerm is allocated or if there is no content.
func (m *Mux) ChildExitOutput() string {
	m.mu.Lock()
	vtm := m.vterm
	m.mu.Unlock()
	if vtm == nil {
		return ""
	}
	return vtm.String()
}

// handleResize is called when the terminal is resized (SIGWINCH).
// It updates the internal dimensions, resizes the VTerm (accounting for
// the status bar), calls the resize callback to propagate to the child
// PTY, and re-renders the status bar.
func (m *Mux) handleResize(rows, cols int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.termRows = rows
	m.termCols = cols

	statusBarLines := 0
	if m.cfg.StatusEnabled && m.statusBar != nil {
		statusBarLines = 1
		m.statusBar.SetHeight(rows)
		m.statusBar.SetScrollRegion()
	}

	childRows := rows - statusBarLines
	if childRows < 1 {
		childRows = 1
	}

	if m.vterm != nil {
		m.vterm.Resize(childRows, cols)
	}

	if m.cfg.ResizeFn != nil {
		_ = m.cfg.ResizeFn(uint16(childRows), uint16(cols))
	}

	if m.cfg.StatusEnabled && m.statusBar != nil {
		m.statusBar.Render()
	}
}

// RunPassthrough enters Claude mode: raw byte forwarding between
// stdin/stdout and the child PTY. This method blocks until:
//   - The user presses the toggle key (ExitToggle)
//   - The child process exits/EOF (ExitChildExit)
//   - The context is cancelled (ExitContext)
//   - An I/O error occurs (ExitError)
//
// Terminal state is saved before entering passthrough and restored after.
// If StatusEnabled is true, a status bar is rendered on the last terminal row.
//
// Returns the exit reason and any associated error. For ExitToggle and
// ExitChildExit, the error is nil.
func (m *Mux) RunPassthrough(ctx context.Context) (ExitReason, error) {
	// ── T050: Precondition checks ──────────────────────────────────
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
	toggleKey := m.cfg.ToggleKey
	statusEnabled := m.cfg.StatusEnabled
	m.passthroughActive = true
	m.active = SideClaude
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		m.passthroughActive = false
		m.active = SideOsm
		m.mu.Unlock()
	}()

	// ── T050: Save terminal state ──────────────────────────────────
	if m.termFd >= 0 {
		savedState, err := m.termState.MakeRaw(m.termFd)
		if err != nil {
			return ExitError, err
		}
		defer func() {
			_ = m.termState.Restore(m.termFd, savedState)
		}()

		// Ensure stdin fd is in blocking mode. Libraries like go-prompt
		// and BubbleTea's cancelreader may leave the fd with O_NONBLOCK
		// set. Go's os.File.Read() does NOT handle EAGAIN for TTY fds.
		origFlags, flagErr := m.blockingGuard.EnsureBlocking(m.termFd)
		if flagErr == nil {
			defer m.blockingGuard.Restore(m.termFd, origFlags)
		}
		// If EnsureBlocking fails, proceed anyway — the EAGAIN retry
		// loop in the stdin goroutine provides defense-in-depth.
	}

	// ── T051: Status bar setup ─────────────────────────────────────
	var statusBarHeight int
	// statusBarLines is 1 when the status bar is rendered (reserving the
	// last terminal row), 0 otherwise. Subtracted from terminal height
	// for VTerm sizing and child PTY resize so the child's scroll
	// behavior matches the real terminal's constrained scroll region.
	var statusBarLines int
	if statusEnabled && m.termFd >= 0 {
		w, h, err := m.termState.GetSize(m.termFd)
		if err == nil && h > 1 {
			statusBarHeight = h
			statusBarLines = 1

			m.statusBar.SetHeight(h)

			// Set scroll region: rows 1..h-1, reserve last row.
			m.mu.Lock()
			m.statusBar.SetScrollRegion()
			m.mu.Unlock()
			defer func() {
				m.mu.Lock()
				m.statusBar.ResetScrollRegion()
				m.mu.Unlock()
			}()

			// Render status bar on the last row.
			m.mu.Lock()
			m.statusBar.Render()
			m.mu.Unlock()

			// Resize VTerm to match constrained scroll region.
			childRows := h - statusBarLines
			m.mu.Lock()
			m.termRows = h
			m.termCols = w
			if m.vterm != nil {
				m.vterm.Resize(childRows, w)
			}
			m.mu.Unlock()
		} else {
			statusEnabled = false
		}
	}

	// Update terminal dimensions even when status bar is off.
	if statusBarLines == 0 && m.termFd >= 0 {
		if w, h, err := m.termState.GetSize(m.termFd); err == nil {
			m.mu.Lock()
			m.termRows = h
			m.termCols = w
			if m.vterm != nil {
				m.vterm.Resize(h, w)
			}
			m.mu.Unlock()
		}
	}

	// ── T052/T053: First-swap clear or VTerm restore ───────────────
	m.mu.Lock()
	firstSwap := !m.swappedOnce
	m.swappedOnce = true
	resizeFn := m.cfg.ResizeFn
	m.mu.Unlock()

	if firstSwap {
		// ESC[2J = erase entire display, ESC[H = cursor to 1,1.
		m.mu.Lock()
		_, _ = m.stdout.Write([]byte("\x1b[2J\x1b[H"))
		m.mu.Unlock()
		// Nudge the child with a resize so it redraws at the correct
		// dimensions (accounting for status bar).
		if resizeFn != nil && m.termFd >= 0 {
			if w, h, err := m.termState.GetSize(m.termFd); err == nil {
				_ = resizeFn(uint16(h-statusBarLines), uint16(w))
			}
		}
	} else {
		// Restore Claude's screen from VTerm buffer — flicker-free.
		// Instead of clearing the screen then redrawing, we overwrite
		// every row in-place with RenderFullScreen (CUP + content + EL).
		m.mu.Lock()
		vtm := m.vterm
		m.mu.Unlock()
		if vtm != nil {
			// Render outside m.mu (VTerm has its own lock), then write
			// to stdout under m.mu to avoid racing with tee goroutine.
			rendered := vtm.RenderFullScreen()
			m.mu.Lock()
			_, _ = m.stdout.Write([]byte(rendered))
			m.mu.Unlock()
		}
		// Re-render status bar after VTerm restore. VTerm content
		// doesn't include status bar bytes.
		if statusBarHeight > 0 {
			m.mu.Lock()
			m.statusBar.Render()
			m.mu.Unlock()
		}
	}

	// ── T119: SIGWINCH resize watcher ──────────────────────────────
	// Start a goroutine that listens for terminal resize signals and
	// propagates them to the VTerm, child PTY, and status bar.
	resizeCtx, resizeCancel := context.WithCancel(ctx)
	defer resizeCancel()
	if m.termFd >= 0 {
		go watchResize(resizeCtx, m.termFd, m.termState, func(rows, cols int) {
			m.handleResize(rows, cols)
		})
	}

	// ── T054: stdin→child forwarding with toggle key detection ─────
	m.mu.Lock()
	childEOF := m.childEOF
	m.mu.Unlock()

	fwdCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	type fwdResult struct {
		reason ExitReason
		err    error
	}
	resultCh := make(chan fwdResult, 1)

	// Goroutine: stdin → child PTY with toggle key interception.
	// The tee goroutine (started by Attach) handles child → stdout.
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
				// Defense-in-depth: retry on EAGAIN even after
				// EnsureBlocking, in case another goroutine re-set
				// O_NONBLOCK.
				if errors.Is(err, syscall.EAGAIN) || errors.Is(err, syscall.EWOULDBLOCK) {
					runtime.Gosched()
					continue
				}
				resultCh <- fwdResult{ExitError, err}
				return
			}
			// Scan for toggle key in the received bytes.
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

	// ── T055: Wait for exit signal ─────────────────────────────────
	select {
	case r := <-resultCh:
		cancel()
		return r.reason, r.err
	case <-childEOF:
		cancel()
		return ExitChildExit, nil
	case <-ctx.Done():
		cancel()
		return ExitContext, ctx.Err()
	}
}
