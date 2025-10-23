package scripting

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// Verify that flush writes messages verbatim as queued and that PrintToTUI provides newlines
func TestFlushQueuedOutput_WithSinkAndWriter_Newlines(t *testing.T) {
	var out bytes.Buffer
	ctx := context.Background()
	eng := mustNewEngine(t, ctx, &out, &out)

	// Create a manager instance that writes to our buffer (not stdout)
	tm := NewTUIManagerWithConfig(context.Background(), eng, nil, &out, "", "")

	// Manually set a sink that appends to queue like Run() would do
	tm.engine.logger.SetTUISink(func(msg string) {
		tm.outputMu.Lock()
		defer tm.outputMu.Unlock()
		tm.outputQueue = append(tm.outputQueue, msg)
	})

	eng.logger.PrintToTUI("line1")
	eng.logger.PrintToTUI("line2\n")

	tm.flushQueuedOutput()

	got := out.String()
	want := "line1\nline2\n"
	if got != want {
		t.Fatalf("flush output mismatch:\n got: %q\nwant: %q", got, want)
	}

	if strings.Contains(got, "\n\n\n") {
		t.Fatalf("unexpected extra newlines in output: %q", got)
	}
}

// TestResetCommand_EndToEnd validates the reset command functionality.
// It verifies that the reset command:
// 1. Clears all shared state and resets to default values
// 2. Clears all mode-specific state and resets to default values for ALL modes
// 3. Preserves the currently active mode
// 4. Returns an error when called with arguments
func TestResetCommand_EndToEnd(t *testing.T) {
	ctx := context.Background()
	var out bytes.Buffer
	engine := mustNewEngine(t, ctx, &out, &out)

	// Get the TUIManager from the engine (which was automatically created)
	tuiManager := engine.GetTUIManager()
	if tuiManager == nil {
		t.Fatal("Expected engine to have a TUIManager")
	}

	// Register modes with mode-specific state
	modesScript := engine.LoadScriptFromString("modes-setup", `
		// Create a shared state contract using the correct API
		const SharedKeys = tui.createSharedStateContract("__shared__", {
			sharedKey: {
				description: "__shared__:sharedKey",
				defaultValue: "defaultShared"
			}
		});

		// Create mode-specific contracts
		const ModeAKeys = tui.createStateContract("modeA", {
			keyA: {
				description: "modeA:keyA",
				defaultValue: 0
			}
		});

		const ModeBKeys = tui.createStateContract("modeB", {
			keyB: {
				description: "modeB:keyB",
				defaultValue: "defaultB"
			}
		});

		// Register a mode with the shared contract first
		tui.registerMode({
			name: "shared-user",
			stateContract: SharedKeys,
			tui: {
				prompt: "[shared]> "
			}
		});

		// Register modeA
		tui.registerMode({
			name: "modeA",
			stateContract: ModeAKeys,
			tui: {
				prompt: "[modeA]> "
			}
		});

		// Register modeB
		tui.registerMode({
			name: "modeB",
			stateContract: ModeBKeys,
			tui: {
				prompt: "[modeB]> "
			}
		});
	`)
	if err := engine.ExecuteScript(modesScript); err != nil {
		t.Fatalf("Failed to register modes: %v", err)
	}

	// Switch to modeA
	if err := tuiManager.SwitchMode("modeA"); err != nil {
		t.Fatalf("Failed to switch to modeA: %v", err)
	}

	// Verify current mode is modeA
	if currentMode := tuiManager.GetCurrentMode(); currentMode == nil || currentMode.Name != "modeA" {
		t.Fatalf("Expected current mode to be modeA, got %v", currentMode)
	}

	// Pre-set state: modify all state values
	if err := tuiManager.SetStateViaJS("__shared__:sharedKey", "modifiedShared"); err != nil {
		t.Fatalf("Failed to set shared state: %v", err)
	}
	if err := tuiManager.SetStateViaJS("modeA:keyA", 100); err != nil {
		t.Fatalf("Failed to set modeA state: %v", err)
	}
	if err := tuiManager.SetStateViaJS("modeB:keyB", "modifiedB"); err != nil {
		t.Fatalf("Failed to set modeB state: %v", err)
	}

	// Verify modified state values
	sharedVal, err := tuiManager.GetStateViaJS("__shared__:sharedKey")
	if err != nil {
		t.Fatalf("Failed to get shared state: %v", err)
	}
	if sharedVal != "modifiedShared" {
		t.Errorf("Expected shared state to be 'modifiedShared', got %v", sharedVal)
	}

	modeAVal, err := tuiManager.GetStateViaJS("modeA:keyA")
	if err != nil {
		t.Fatalf("Failed to get modeA state: %v", err)
	}
	if modeAVal != int64(100) {
		t.Errorf("Expected modeA state to be 100, got %v", modeAVal)
	}

	modeBVal, err := tuiManager.GetStateViaJS("modeB:keyB")
	if err != nil {
		t.Fatalf("Failed to get modeB state: %v", err)
	}
	if modeBVal != "modifiedB" {
		t.Errorf("Expected modeB state to be 'modifiedB', got %v", modeBVal)
	}

	// Execute the reset command
	err = tuiManager.ExecuteCommand("reset", []string{})
	if err != nil {
		t.Fatalf("Expected reset command to succeed, got error: %v", err)
	}

	// Verify current mode is still modeA (mode should not change)
	if currentMode := tuiManager.GetCurrentMode(); currentMode == nil || currentMode.Name != "modeA" {
		t.Fatalf("Expected current mode to still be modeA after reset, got %v", currentMode)
	}

	// Verify all state has been reset to default values
	sharedVal, err = tuiManager.GetStateViaJS("__shared__:sharedKey")
	if err != nil {
		t.Fatalf("Failed to get shared state after reset: %v", err)
	}
	if sharedVal != "defaultShared" {
		t.Errorf("Expected shared state to be reset to 'defaultShared', got %v", sharedVal)
	}

	modeAVal, err = tuiManager.GetStateViaJS("modeA:keyA")
	if err != nil {
		t.Fatalf("Failed to get modeA state after reset: %v", err)
	}
	if modeAVal != int64(0) {
		t.Errorf("Expected modeA state to be reset to 0, got %v", modeAVal)
	}

	// Switch to modeB to check its state was also reset
	if err := tuiManager.SwitchMode("modeB"); err != nil {
		t.Fatalf("Failed to switch to modeB: %v", err)
	}

	modeBVal, err = tuiManager.GetStateViaJS("modeB:keyB")
	if err != nil {
		t.Fatalf("Failed to get modeB state after reset: %v", err)
	}
	if modeBVal != "defaultB" {
		t.Errorf("Expected modeB state to be reset to 'defaultB', got %v", modeBVal)
	}

	// Switch back to modeA for the final checks
	if err := tuiManager.SwitchMode("modeA"); err != nil {
		t.Fatalf("Failed to switch back to modeA: %v", err)
	}

	// Verify invalid usage: reset with arguments should fail
	err = tuiManager.ExecuteCommand("reset", []string{"invalid-arg"})
	if err == nil {
		t.Fatal("Expected reset command with arguments to return an error, got nil")
	}
	if !strings.Contains(err.Error(), "usage: reset") {
		t.Errorf("Expected error message to contain 'usage: reset', got: %v", err)
	}
}
