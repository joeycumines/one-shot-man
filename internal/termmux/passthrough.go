package termmux

import (
	"context"
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

	resultCh := make(chan forwardResult, 1)

	// Build pre-processor for SGR mouse filtering (status bar click interception).
	var preProcess func(data []byte, carry []byte) (filtered []byte, newCarry []byte, clicked bool)
	if statusBarLines > 0 && cfg.TermFd >= 0 && cfg.TermState != nil {
		preProcess = func(data []byte, carry []byte) ([]byte, []byte, bool) {
			// Prepend carry-over bytes from a previous partial SGR prefix.
			if len(carry) > 0 {
				data = append(carry, data...)
			}
			_, th, _ := cfg.TermState.GetSize(cfg.TermFd)
			filtered, partial, clicked := filterMouseForStatusBar(data, th, statusBarLines)
			return filtered, partial, clicked
		}
	}

	go forwardStdin(fwdCtx, resultCh, forwardConfig{
		Stdin:      cfg.Stdin,
		Writer:     w,
		ToggleKey:  cfg.ToggleKey,
		PreProcess: preProcess,
	})

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
