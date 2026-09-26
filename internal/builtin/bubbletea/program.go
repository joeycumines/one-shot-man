package bubbletea

import (
	"context"
	"fmt"
	"os"
	"runtime/debug"
	"sync"
	"syscall"

	tea "charm.land/bubbletea/v2"
	"golang.org/x/term"
)

// WaitForProgram blocks until the currently running BubbleTea program exits.
// Returns nil if no program is running. This is used by callers that need to
// block after starting a non-blocking tea.run() — for example, ExecuteScript
// routes the wizard launch through the event loop (so the Goja VM is only
// accessed from a single goroutine), and then WaitForProgram keeps the
// command goroutine alive until BubbleTea exits.
func (m *Manager) WaitForProgram() error {
	m.mu.Lock()
	done := m.programDone
	m.mu.Unlock()

	if done == nil {
		return nil
	}

	err := <-done

	m.mu.Lock()
	m.programDone = nil
	m.mu.Unlock()

	return err
}

// SendStateRefresh sends a state refresh message to the currently running program.
// This is safe to call from any goroutine. If no program is running, it's a no-op.
// The key parameter indicates which state key changed (for debugging/logging).
func (m *Manager) SendStateRefresh(key string) {
	m.mu.Lock()
	p := m.program
	m.mu.Unlock()

	if p != nil {
		p.Send(stateRefreshMsg{key: key})
	}
}

// runProgram runs a bubbletea program with the given model.
// Handles TTY detection, terminal state cleanup, panic recovery, and proper I/O setup.
// If a panic occurs, the terminal is restored before printing the stack trace to stderr.
//
// In v2, terminal features (altScreen, mouse, etc.) are controlled declaratively
// via tea.View fields returned by the model's View() method, not via program options.
// unwrapOSFile extracts an *os.File from v, checking first for a direct
// *os.File and then for the UnwrapFile() interface used by wrapper types
// (e.g., TUIReader/TUIWriter).  Returns nil if v is nil or not unwrappable.
func unwrapOSFile(v any) *os.File {
	if v == nil {
		return nil
	}
	if f, ok := v.(*os.File); ok {
		return f
	}
	if u, ok := v.(interface{ UnwrapFile() *os.File }); ok {
		return u.UnwrapFile()
	}
	return nil
}

func (m *Manager) runProgram(model tea.Model) (err error) {
	// Debug: check if manager is properly initialized
	if m == nil {
		return fmt.Errorf("runProgram: manager is nil")
	}
	if m.ctx == nil {
		return fmt.Errorf("runProgram: manager.ctx is nil")
	}

	// Enable bracketed paste mode so \x1b[200~...\x1b[201~ sequences are
	// recognized as paste events (tea.PasteStartMsg/PasteEndMsg/PasteMsg).
	// We send this to the terminal here, before BubbleTea's read loop starts.
	// Note: bracketed paste mode is always enabled; the jsModel-level field was
	// removed as it was dead code (never read, only written).
	if m.output != nil {
		_, _ = m.output.Write([]byte("\x1b[?2004h"))
	}

	ctx, cancel := context.WithCancelCause(m.ctx)
	defer cancel(nil)

	// Lock only for accessing/copying configuration, not during p.Run()
	m.mu.Lock()
	input := m.input
	output := m.output
	stderr := m.stderr
	signalNotify := m.signalNotify
	signalStop := m.signalStop
	isTTY := m.isTTY
	ttyFd := m.ttyFd
	if m.program != nil {
		m.mu.Unlock()
		return fmt.Errorf("runProgram: program is already running")
	}
	m.mu.Unlock()

	// Debug: validate required function pointers
	if signalNotify == nil {
		return fmt.Errorf("runProgram: signalNotify is nil")
	}
	if signalStop == nil {
		return fmt.Errorf("runProgram: signalStop is nil")
	}

	// Save terminal state for cleanup
	var origState *term.State
	if isTTY && ttyFd >= 0 {
		var stateErr error
		origState, stateErr = term.GetState(ttyFd)
		if stateErr != nil {
			// Non-fatal: we just won't be able to restore state
			origState = nil
		}
	}

	// restoreTerminal restores the terminal state and disables bracketed paste mode.
	restoreTerminal := func() {
		// Disable bracketed paste mode (DECRST 200)
		if m.output != nil {
			_, _ = m.output.Write([]byte("\x1b[?2004l"))
		}
		if origState != nil && ttyFd >= 0 {
			_ = term.Restore(ttyFd, origState)
		}
	}

	// Ensure terminal state is restored even on panic
	// Also capture and log the panic with stack trace
	defer func() {
		if r := recover(); r != nil {
			// FIRST: Restore terminal to ensure output is readable
			restoreTerminal()

			// Get the full stack trace
			stackTrace := debug.Stack()

			// Log the panic to stderr with full details
			panicMsg := fmt.Sprintf("\n[%s] PANIC in bubbletea program:\n  %v\n\nStack Trace:\n%s\n",
				ErrCodePanic, r, string(stackTrace))

			if stderr != nil {
				_, _ = fmt.Fprint(stderr, panicMsg)
			}

			// Convert panic to error for the caller
			err = fmt.Errorf("%s: panic recovered: %v", ErrCodePanic, r)
		} else {
			// Normal exit - still restore terminal
			restoreTerminal()
		}
	}()

	// Configure input/output.
	// In v2, BubbleTea opens /dev/tty for input when no explicit input is provided
	// via WithInput.  We need to pass the real *os.File so BubbleTea can detect
	// TTY capabilities (raw mode, signals, etc.).
	//
	// Input/output may be wrapper types (e.g., TUIReader) that aren't *os.File
	// but wrap one.  We check for the UnwrapFile() interface to recover the
	// underlying file in those cases.
	var opts []tea.ProgramOption
	if f := unwrapOSFile(input); f != nil {
		opts = append(opts, tea.WithInput(f))
	} else if input != nil {
		opts = append(opts, tea.WithInput(input))
	}
	if f := unwrapOSFile(output); f != nil {
		opts = append(opts, tea.WithOutput(f))
	} else if output != nil {
		opts = append(opts, tea.WithOutput(output))
	}
	// Signal ownership belongs to the embedding runtime, not the program:
	// bubbletea's own handler turns SIGTERM into QuitMsg and SIGINT into
	// InterruptMsg, stopping the program behind the script's back. A script
	// that listens (the engine delivers SIGINT/SIGTERM to process listeners,
	// Node-style) then never gets to run its event-driven drain, and a script
	// that does not is still covered because the engine cancels the runtime,
	// which lands on the ctx.Done() arm of the watchdog goroutine below.
	opts = append(opts, tea.WithoutSignalHandler())

	p := tea.NewProgram(model, opts...)

	// Store program reference for external message injection (e.g., state refresh)
	m.mu.Lock()
	m.program = p
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		m.program = nil
		m.mu.Unlock()
	}()

	// Also store program reference in jsModel for render throttling
	// If the model is wrapped in a toggleModel, unwrap to reach the jsModel
	// and set the program reference on the toggle wrapper too.
	actualModel := model
	if tm, ok := model.(*toggleModel); ok {
		tm.mu.Lock()
		tm.program = p
		tm.mu.Unlock()
		actualModel = tm.inner
		defer func() {
			tm.mu.Lock()
			tm.program = nil
			tm.mu.Unlock()
		}()
	}
	if jm, ok := actualModel.(*jsModel); ok {
		// Set up throttle cancellation context to prevent goroutine leak
		// when program exits while a throttle timer is sleeping
		throttleCtx, throttleCancel := context.WithCancel(ctx)
		jm.throttleCtx = throttleCtx
		jm.throttleCancel = throttleCancel
		jm.program = p
		defer func() {
			// Cancel any pending throttle timers FIRST, then nil program
			throttleCancel()
			jm.throttleCtx = nil
			jm.throttleCancel = nil
			jm.program = nil
		}()
	}

	// Setup signal handling
	sigCh := make(chan os.Signal, 1)
	signalNotify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	defer signalStop(sigCh)

	// Channel to signal that Run() has finished
	programFinished := make(chan struct{})

	var wg sync.WaitGroup
	wg.Go(func() {
		for {
			select {
			case <-programFinished:
				// Program finished naturally, no need to call Quit
				return
			case <-ctx.Done():
				// Context cancelled externally: the runtime is being torn down
				// (engine shutdown, or the engine's unhandled-signal fallback),
				// so end the program and restore the terminal.
			case sig := <-sigCh:
				// SIGINT/SIGTERM belong to the script: the engine delivers them
				// to the script's Node-style listener, and the script decides
				// when the program ends (typically after an event-driven drain).
				// Quitting here would stop the program behind the script's back,
				// so its drain never observes the child exit and the process
				// lingers. A script with no listener is force-cancelled by the
				// engine instead, which lands on the ctx.Done() arm above — so
				// keep watching rather than returning, or that arm is lost.
				// SIGQUIT has no engine contract; keep the terminal-restoring
				// quit.
				if sig != syscall.SIGQUIT {
					continue
				}
			}
			p.Quit()
			return
		}
	})

	_, runErr := p.Run()
	close(programFinished) // Signal that Run() returned
	cancel(nil)            // Signal the goroutine to exit (via ctx.Done path if race, but programFinished priority)
	wg.Wait()              // Wait for the goroutine to finish

	if runErr != nil {
		return fmt.Errorf("failed to run program: %w", runErr)
	}

	return nil
}
