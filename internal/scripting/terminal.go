package scripting

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
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
}
