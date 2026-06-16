package termmux

import (
	"context"
	"fmt"

	"github.com/joeycumines/one-shot-man/internal/termmux/statusbar"
)

// Passthrough enters direct terminal I/O mode for the active session.
// It takes over the terminal for low-latency stdin→PTY and PTY→stdout
// forwarding while the worker goroutine continues processing output
// through VTerm for snapshot capture.
//
// Passthrough runs in the caller's goroutine (outside the worker). It
// communicates with the worker via requests to get the active session's
// writer and to enable/disable the output tee.
//
// Returns the reason for exiting passthrough and any associated error.
func (m *SessionManager) Passthrough(ctx context.Context, cfg PassthroughConfig) (ExitReason, error) {
	// ── Precondition: active session must exist ─────────────────────
	// The writer is captured once at entry and does not change even
	// if the active session is switched during passthrough.
	w, err := m.activeWriter()
	if err != nil {
		return ExitError, err
	}

	// ── Terminal raw mode ───────────────────────────────────────────
	if cfg.TermFd >= 0 && cfg.TermState != nil {
		savedState, rawErr := cfg.TermState.MakeRaw(cfg.TermFd)
		if rawErr != nil {
			return ExitError, rawErr
		}
		defer func() {
			_ = cfg.TermState.Restore(cfg.TermFd, savedState)
		}()

		// Ensure stdin fd is in blocking mode. Go's os.File.Read does
		// NOT handle EAGAIN for TTY fds. Defense-in-depth EAGAIN retry
		// is in the stdin goroutine below.
		if cfg.BlockingGuard != nil {
			origFlags, flagErr := cfg.BlockingGuard.EnsureBlocking(cfg.TermFd)
			if flagErr == nil {
				defer cfg.BlockingGuard.Restore(cfg.TermFd, origFlags)
			}
		}
	}

	// ── Precondition: active session must exist ─────────────────────
	activeID := m.ActiveID()
	if activeID == 0 {
		return ExitError, ErrSessionNotFound
	}

	// ── Status bar and message overlay setup ────────────────────────
	var statusBarLines int
	var messageBarLines int
	var chromeRows int
	var termRows, termCols int
	var vtermRows int // actual VTerm row count after resize
	if cfg.StatusBar != nil && cfg.TermFd >= 0 && cfg.TermState != nil {
		w2, h, sizeErr := cfg.TermState.GetSize(cfg.TermFd)
		if sizeErr == nil && h > 1 {
			termRows, termCols = h, w2
			statusBarLines = 1
			if m.ActiveMessage(activeID) != "" {
				messageBarLines = 1
			}
			chromeRows = statusBarLines + messageBarLines

			cfg.StatusBar.SetHeight(h)
			setChromeScrollRegion(cfg.StatusBar, chromeRows)
			defer cfg.StatusBar.ResetScrollRegion()

			renderChrome(cfg, m, activeID, termRows, termCols, chromeRows, statusBarLines)

			// Resize all sessions' VTerms to account for chrome.
			childRows := max(h-chromeRows, 1)
			_ = m.Resize(childRows, w2)
			vtermRows = childRows
		}
	}

	// If no status bar, still update terminal dimensions.
	if statusBarLines == 0 && cfg.TermFd >= 0 && cfg.TermState != nil {
		if w2, h, sizeErr := cfg.TermState.GetSize(cfg.TermFd); sizeErr == nil {
			termRows, termCols = h, w2
			_ = m.Resize(h, w2)
			vtermRows = h
		}
	}

	// ── Screen display: clear or restore ────────────────────────────
	if cfg.RestoreScreen {
		// Restore the active session's VTerm screen in-place.
		snap := m.Snapshot(activeID)
		if snap != nil && snap.GetFullScreen() != "" {
			if err := writeOrLog(cfg.Stdout, []byte(snap.GetFullScreen()), "vterm-restore"); err != nil {
				return ExitError, err
			}
		}
		// Erase rows beyond the VTerm's height to prevent ghost
		// content from a previous session persisting below the
		// restored screen. Move cursor to row after VTerm content
		// and erase to end of screen (ED mode 0).
		// Use the actual VTerm row count, not snap.Rows, which may
		// be stale if the resize hasn't produced a new snapshot yet.
		eraseRow := vtermRows
		if eraseRow == 0 && snap != nil {
			eraseRow = snap.Rows
		}
		if eraseRow > 0 {
			if err := writeOrLog(cfg.Stdout, fmt.Appendf(nil, "\x1b[%d;1H\x1b[0J", eraseRow+1), "vterm-erase-below"); err != nil {
				return ExitError, err
			}
		}
		// Re-render chrome after VTerm restore.
		if cfg.StatusBar != nil && statusBarLines > 0 {
			renderChrome(cfg, m, activeID, termRows, termCols, chromeRows, statusBarLines)
		}
	} else {
		// First swap: clear screen + home cursor.
		if err := writeOrLog(cfg.Stdout, []byte("\x1b[2J\x1b[H"), "first-swap-clear"); err != nil {
			return ExitError, err
		}
	}

	// Nudge the child with a resize so it redraws at the correct
	// dimensions (accounting for chrome).
	if cfg.ResizeFn != nil && cfg.TermFd >= 0 && cfg.TermState != nil {
		if w2, h, sizeErr := cfg.TermState.GetSize(cfg.TermFd); sizeErr == nil {
			childH := max(h-chromeRows, 1)
			_ = cfg.ResizeFn(uint16(childH), uint16(w2))
		}
	}

	// ── Enable output tee: PTY → stdout ─────────────────────────────
	if teeErr := m.enablePassthroughTee(activeID, cfg.Stdout); teeErr != nil {
		return ExitError, teeErr
	}
	defer func() { _ = m.disablePassthroughTee() }()

	// ── Lock overlay ────────────────────────────────────────────────
	// If the active session is locked at entry, draw the unlock prompt
	// before forwarding input. The overlay anchors to the terminal size
	// so the user sees a masked password prompt rather than session
	// content underneath.
	if pr := m.UnlockPrompt(activeID); pr.active {
		rows, cols := m.termRows, m.termCols
		if cfg.TermFd >= 0 && cfg.TermState != nil {
			if w2, h, sizeErr := cfg.TermState.GetSize(cfg.TermFd); sizeErr == nil {
				rows, cols = h, w2
			}
		}
		_ = RenderUnlockPrompt(cfg.Stdout, pr.maskLen, pr.message, rows, cols)
	}

	// ── SIGWINCH resize watcher ─────────────────────────────────────
	resizeCtx, resizeCancel := context.WithCancel(ctx)
	defer resizeCancel()
	if cfg.TermFd >= 0 && cfg.TermState != nil {
		go watchResize(resizeCtx, cfg.TermFd, cfg.TermState, func(rows, cols int) {
			newMessageBarLines := 0
			if cfg.StatusBar != nil && statusBarLines > 0 && m.ActiveMessage(activeID) != "" {
				newMessageBarLines = 1
			}
			newChromeRows := statusBarLines + newMessageBarLines

			childRows := max(rows-newChromeRows, 1)
			_ = m.Resize(childRows, cols)

			if cfg.StatusBar != nil && statusBarLines > 0 {
				cfg.StatusBar.SetHeight(rows)
				setChromeScrollRegion(cfg.StatusBar, newChromeRows)
				renderChrome(cfg, m, activeID, rows, cols, newChromeRows, statusBarLines)
			}

			if cfg.ResizeFn != nil {
				_ = cfg.ResizeFn(uint16(childRows), uint16(cols))
			}
		})
	}

	// ── stdin→PTY forwarding with toggle key detection ──────────────
	// Subscribe to session events so we can detect child exit.
	subID, evtCh := m.Subscribe(16)
	defer m.Unsubscribe(subID)

	fwdCtx, fwdCancel := context.WithCancel(ctx)
	defer fwdCancel()

	resultCh := make(chan forwardResult, 1)

	// Build pre-processor chain: pane key interception → SGR mouse filtering.
	var preProcess func(data []byte, carry []byte) (filtered []byte, newCarry []byte, clicked bool)
	if cfg.Handler != nil || (statusBarLines > 0 && cfg.TermFd >= 0 && cfg.TermState != nil) {
		preProcess = func(data []byte, carry []byte) ([]byte, []byte, bool) {
			// Prepend carry-over bytes from a previous partial prefix.
			if len(carry) > 0 {
				data = append(carry, data...)
			}

			// Pane key interception takes priority.
			if cfg.Handler != nil {
				prefixLen, action, remaining := cfg.Handler.HandleKeyInBuffer(data)
				if prefixLen > 0 {
					if cfg.OnAction != nil {
						cfg.OnAction(action)
					}
					data = remaining
					// If the pane key consumed the entire buffer, return empty.
					if len(data) == 0 {
						return nil, nil, false
					}
					// Fall through to SGR mouse filtering with remaining data.
				}
			}

			if chromeRows > 0 && cfg.TermFd >= 0 && cfg.TermState != nil {
				_, th, _ := cfg.TermState.GetSize(cfg.TermFd)
				filtered, partial, clicked := filterMouseForStatusBar(data, th, chromeRows)
				return filtered, partial, clicked
			}

			return data, nil, false
		}
	}

	go forwardStdin(fwdCtx, resultCh, forwardConfig{
		Stdin:      cfg.Stdin,
		Writer:     w,
		ToggleKey:  cfg.ToggleKey,
		PreProcess: preProcess,
	})

	// ── Signal forwarding (SIGINT, SIGQUIT, SIGTSTP) ────────────────
	sigResultCh := make(chan signalResult, 1)
	sigCancel := watchSignals(ctx, sigResultCh, cfg.SignalChild)
	defer sigCancel()

	// ── Wait for exit signal ────────────────────────────────────────
	for {
		select {
		case r := <-resultCh:
			fwdCancel()
			// When context cancellation and a result arrive simultaneously
			// (e.g., context cancel kills the child, which produces a result),
			// prioritize context cancellation for consistency with
			// CaptureSession.Passthrough.
			if ctx.Err() != nil {
				return ExitContext, ctx.Err()
			}
			return r.reason, r.err
		case evt := <-evtCh:
			if evt.Kind == EventSessionExited && evt.SessionID == activeID {
				fwdCancel()
				return ExitChildExit, nil
			}
			if evt.Kind == EventSessionClosed && evt.SessionID == activeID {
				fwdCancel()
				return ExitChildExit, nil
			}
		case sr := <-sigResultCh:
			sigCancel()
			fwdCancel()
			return sr.reason, sr.err
		case <-ctx.Done():
			fwdCancel()
			return ExitContext, ctx.Err()
		}
	}
}

// setChromeScrollRegion configures the terminal scroll region to exclude the
// chrome rows (status bar plus optional message bar).
func setChromeScrollRegion(sb *statusbar.StatusBar, chromeRows int) {
	sb.SetScrollRegionEx(chromeRows)
}

func renderChrome(cfg PassthroughConfig, m *SessionManager, activeID SessionID, termRows, termCols, chromeRows, statusBarLines int) {
	if cfg.StatusBar == nil || statusBarLines == 0 || termCols <= 0 || termRows <= 0 {
		return
	}
	if chromeRows > statusBarLines {
		msgRow := messageBarRow(termRows, cfg.StatusBar.Position())
		_ = RenderMessageBar(cfg.Stdout, m.ActiveMessage(activeID), msgRow, termCols)
	}
	cfg.StatusBar.Render()
}

// messageBarRow returns the 1-based terminal row for the message bar given
// the status bar position. The message bar is drawn adjacent to the status
// bar on the side closest to the pane content.
func messageBarRow(termRows int, pos statusbar.Position) int {
	if pos == statusbar.PositionTop {
		return 2
	}
	return termRows - 1
}
