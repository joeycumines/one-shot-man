package termmux

import (
	"context"
	"errors"
	"runtime"
	"syscall"
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

	// ── Status bar setup ────────────────────────────────────────────
	var statusBarLines int
	if cfg.StatusBar != nil && cfg.TermFd >= 0 && cfg.TermState != nil {
		w2, h, sizeErr := cfg.TermState.GetSize(cfg.TermFd)
		if sizeErr == nil && h > 1 {
			statusBarLines = 1

			cfg.StatusBar.SetHeight(h)
			cfg.StatusBar.SetScrollRegion()
			defer cfg.StatusBar.ResetScrollRegion()

			cfg.StatusBar.Render()

			// Resize all sessions' VTerms to account for the status bar.
			childRows := h - statusBarLines
			_ = m.Resize(childRows, w2)
		}
	}

	// If no status bar, still update terminal dimensions.
	if statusBarLines == 0 && cfg.TermFd >= 0 && cfg.TermState != nil {
		if w2, h, sizeErr := cfg.TermState.GetSize(cfg.TermFd); sizeErr == nil {
			_ = m.Resize(h, w2)
		}
	}

	// ── Screen display: clear or restore ────────────────────────────
	activeID := m.ActiveID()
	if cfg.RestoreScreen {
		// Restore the active session's VTerm screen in-place.
		snap := m.Snapshot(activeID)
		if snap != nil && snap.FullScreen != "" {
			writeOrLog(cfg.Stdout, []byte(snap.FullScreen), "vterm-restore")
		}
		// Re-render status bar after VTerm restore.
		if cfg.StatusBar != nil && statusBarLines > 0 {
			cfg.StatusBar.Render()
		}
	} else {
		// First swap: clear screen + home cursor.
		writeOrLog(cfg.Stdout, []byte("\x1b[2J\x1b[H"), "first-swap-clear")
	}

	// Nudge the child with a resize so it redraws at the correct
	// dimensions (accounting for status bar).
	if cfg.ResizeFn != nil && cfg.TermFd >= 0 && cfg.TermState != nil {
		if w2, h, sizeErr := cfg.TermState.GetSize(cfg.TermFd); sizeErr == nil {
			childH := max(h-statusBarLines, 1)
			_ = cfg.ResizeFn(uint16(childH), uint16(w2))
		}
	}

	// ── Enable output tee: PTY → stdout ─────────────────────────────
	if teeErr := m.enablePassthroughTee(activeID, cfg.Stdout); teeErr != nil {
		return ExitError, teeErr
	}
	defer func() { _ = m.disablePassthroughTee() }()

	// ── SIGWINCH resize watcher ─────────────────────────────────────
	resizeCtx, resizeCancel := context.WithCancel(ctx)
	defer resizeCancel()
	if cfg.TermFd >= 0 && cfg.TermState != nil {
		go watchResize(resizeCtx, cfg.TermFd, cfg.TermState, func(rows, cols int) {
			childRows := max(rows-statusBarLines, 1)
			_ = m.Resize(childRows, cols)

			if cfg.StatusBar != nil && statusBarLines > 0 {
				cfg.StatusBar.SetHeight(rows)
				cfg.StatusBar.SetScrollRegion()
				cfg.StatusBar.Render()
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

	type fwdResult struct {
		reason ExitReason
		err    error
	}
	resultCh := make(chan fwdResult, 1)

	go func() {
		buf := make([]byte, 4096)
		for {
			select {
			case <-fwdCtx.Done():
				return
			default:
			}
			n, readErr := cfg.Stdin.Read(buf)
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
				resultCh <- fwdResult{ExitError, readErr}
				return
			}

			data := buf[:n]

			// SGR mouse filtering: intercept clicks on the status bar.
			if statusBarLines > 0 && cfg.TermFd >= 0 && cfg.TermState != nil {
				_, th, _ := cfg.TermState.GetSize(cfg.TermFd)
				filtered, clicked := filterMouseForStatusBar(data, th, statusBarLines)
				if clicked {
					if len(filtered) > 0 {
						writeOrLog(w, filtered, "session-pre-toggle-click")
					}
					resultCh <- fwdResult{ExitToggle, nil}
					return
				}
				data = filtered
			}

			// Toggle key scan.
			for i := 0; i < len(data); i++ {
				if data[i] == cfg.ToggleKey {
					if i > 0 {
						writeOrLog(w, data[:i], "session-pre-toggle-key")
					}
					resultCh <- fwdResult{ExitToggle, nil}
					return
				}
			}

			// Forward all bytes to the active session.
			if _, writeErr := w.Write(data); writeErr != nil {
				if fwdCtx.Err() != nil {
					return
				}
				resultCh <- fwdResult{ExitError, writeErr}
				return
			}
		}
	}()

	// ── Wait for exit signal ────────────────────────────────────────
	for {
		select {
		case r := <-resultCh:
			fwdCancel()
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
		case <-ctx.Done():
			fwdCancel()
			return ExitContext, ctx.Err()
		}
	}
}
