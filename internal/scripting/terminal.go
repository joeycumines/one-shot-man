package scripting

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

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

	// Run TUI in a goroutine so we can handle signals
	done := make(chan struct{})
	go func() {
		defer close(done)
		t.tuiManager.Run()
	}()

	// Wait for either TUI completion or a signal
	select {
	case <-done:
		// TUI exited normally (e.g., 'exit' command or Ctrl+D).
	case sig := <-sigChan:
		// Signal received. Trigger a graceful exit of the prompt.
		_, _ = fmt.Fprintf(t.tuiManager.writer, "\n\nReceived signal %v, shutting down...\n", sig)
		t.tuiManager.TriggerExit()
		<-done // Wait for the TUI goroutine to fully stop.
	}

	// Persist session on ANY exit path (clean or signal-based).
	// This centralizes the save logic.
	if t.tuiManager.stateManager != nil {
		fmt.Fprintln(t.tuiManager.writer, "Saving session...")
		if err := t.tuiManager.stateManager.PersistSession(); err != nil {
			_, _ = fmt.Fprintf(t.tuiManager.writer, "Warning: Failed to persist session: %v\n", err)
		} else {
			fmt.Fprintln(t.tuiManager.writer, "Session saved successfully.")
		}
	}

	// Ensure resources are released on all exit paths.
	if err := t.tuiManager.Close(); err != nil {
		_, _ = fmt.Fprintf(t.tuiManager.writer, "Warning: Failed to close TUI manager: %v\n", err)
	}

	// Restore terminal state after all TUI operations complete.
	// This ensures the terminal is in a clean state regardless of what
	// bubbletea, go-prompt, or other subsystems may have done.
	if origTermState != nil {
		_ = term.Restore(int(os.Stdin.Fd()), origTermState)
	}
}
