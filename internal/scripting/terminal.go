package scripting

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/term"
)

// Terminal provides interactive terminal capabilities for the scripting engine using rich TUI.
type Terminal struct {
	engine     *Engine
	tuiManager *TUIManager
	ctx        context.Context
}

// NewTerminal creates a new terminal interface for the scripting engine.
func NewTerminal(ctx context.Context, engine *Engine) *Terminal {
	return &Terminal{
		engine:     engine,
		tuiManager: engine.GetTUIManager(),
		ctx:        ctx,
	}
}

// Run starts the interactive terminal with rich TUI support.
// Phase 4.5: Added signal handling for graceful shutdown and state persistence.
func (t *Terminal) Run() {
	// Diagnostic trace file for debugging exit hangs. Enabled via
	// OSM_EXIT_TRACE environment variable (set to a file path).
	var traceExit func(string)
	if tracePath := os.Getenv("OSM_EXIT_TRACE"); tracePath != "" {
		f, err := os.OpenFile(tracePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err == nil {
			defer f.Close()
			traceExit = func(msg string) {
				_, _ = fmt.Fprintf(f, "[%s] %s\n", time.Now().Format("15:04:05.000"), msg)
				_ = f.Sync()
			}
		}
	}
	if traceExit == nil {
		traceExit = func(string) {} // no-op
	}

	traceExit("Terminal.Run() starting")

	// Save terminal state before starting TUI operations
	// This ensures we can restore the terminal to a clean state on exit
	var origTermState *term.State
	if term.IsTerminal(int(os.Stdin.Fd())) {
		var err error
		origTermState, err = term.GetState(int(os.Stdin.Fd()))
		if err != nil {
			origTermState = nil // Fallback: can't save state, but still try to run
		}
	}

	// Set up signal handling for SIGINT and SIGTERM
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	// Start the TUI manager in the event loop.
	// TUIManager.Run() uses Promisify to keep the loop alive.
	traceExit("tuiManager.Run() starting")
	t.tuiManager.Run()

	// Wait for the event loop to naturally quiesce (or be canceled by signal).
	// This replaces the manual done channel and select loop.
	go func() {
		sig := <-sigChan
		// Signal received. Trigger a graceful exit of the prompt.
		traceExit(fmt.Sprintf("signal received: %v", sig))
		_, _ = fmt.Fprintf(t.tuiManager.writer, "\n\nReceived signal %v, shutting down...\n", sig)
		t.tuiManager.TriggerExit()
	}()

	t.engine.Wait()
	traceExit("Engine wait complete (loop exited)")

	// Persist session on ANY exit path (clean or signal-based).
	// This centralizes the save logic.
	traceExit("starting session persistence")
	if t.tuiManager.stateManager != nil {
		_, _ = fmt.Fprintln(t.tuiManager.writer, "Saving session...")
		if err := t.tuiManager.stateManager.PersistSession(); err != nil {
			_, _ = fmt.Fprintf(t.tuiManager.writer, "Warning: Failed to persist session: %v\n", err)
			traceExit(fmt.Sprintf("PersistSession error: %v", err))
		} else {
			_, _ = fmt.Fprintln(t.tuiManager.writer, "Session saved successfully.")
			traceExit("PersistSession OK")
		}
	} else {
		traceExit("no stateManager, skip persistence")
	}

	// Ensure resources are released on all exit paths.
	traceExit("starting TUI manager Close()")
	if err := t.tuiManager.Close(); err != nil {
		_, _ = fmt.Fprintf(t.tuiManager.writer, "Warning: Failed to close TUI manager: %v\n", err)
		traceExit(fmt.Sprintf("Close error: %v", err))
	} else {
		traceExit("Close() OK")
	}

	// Restore terminal state after all TUI operations complete.
	// This ensures the terminal is in a clean state regardless of what
	// bubbletea, go-prompt, or other subsystems may have done.
	if origTermState != nil {
		_ = term.Restore(int(os.Stdin.Fd()), origTermState)
	}
	traceExit("Terminal.Run() complete")
}
