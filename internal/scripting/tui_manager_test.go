package scripting

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/testutil"
)

// Verify that flush writes messages verbatim as queued and that PrintToTUI provides newlines
func TestFlushQueuedOutput_WithSinkAndWriter_Newlines(t *testing.T) {
	var out bytes.Buffer
	ctx := context.Background()
	eng := mustNewEngine(t, ctx, &out, &out)

	// Create a manager instance that writes to our buffer (not stdout)
	tm := NewTUIManagerWithConfig(context.Background(), eng, nil, &out, testutil.NewTestSessionID("test-flush", t.Name()), "memory")

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
		// Import shared symbols
		const shared = require('osm:sharedStateSymbols');

		// Create shared state
		const SharedStateKeys = {
			sharedKey: Symbol("sharedKey")
		};
		const sharedState = tui.createState("__shared__", {
			[SharedStateKeys.sharedKey]: {defaultValue: "defaultShared"}
		});

		// Create mode-specific state
		const ModeAKeys = {
			keyA: Symbol("keyA")
		};
		const modeAState = tui.createState("modeA", {
			[ModeAKeys.keyA]: {defaultValue: 0}
		});

		const ModeBKeys = {
			keyB: Symbol("keyB")
		};
		const modeBState = tui.createState("modeB", {
			[ModeBKeys.keyB]: {defaultValue: "defaultB"}
		});

		// Register a mode with the shared state first
		tui.registerMode({
			name: "shared-user",
			tui: {
				prompt: "[shared]> "
			}
		});

		// Register modeA
		tui.registerMode({
			name: "modeA",
			tui: {
				prompt: "[modeA]> "
			}
		});

		// Register modeB
		tui.registerMode({
			name: "modeB",
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
	if modeAVal != 100 {
		t.Errorf("Expected modeA state to be 100, got %v (type %T)", modeAVal, modeAVal)
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
	// NOTE: We verify this by running JS, because the raw storage should be empty (nil),
	// and the JS accessor is responsible for providing the default.

	// 1. Verify raw storage is empty (nil)
	sharedVal, err = tuiManager.GetStateViaJS("__shared__:sharedKey")
	if err != nil {
		t.Fatalf("Failed to get shared state after reset: %v", err)
	}
	if sharedVal != nil {
		t.Errorf("Expected raw shared state to be nil (cleared), got %v", sharedVal)
	}

	modeAVal, err = tuiManager.GetStateViaJS("modeA:keyA")
	if err != nil {
		t.Fatalf("Failed to get modeA state after reset: %v", err)
	}
	if modeAVal != nil {
		t.Errorf("Expected raw modeA state to be nil (cleared), got %v", modeAVal)
	}

	// 2. Verify JS sees the default values
	// We re-create the state accessors to check the values. This works because createState
	// is stateless regarding the accessor itself; it just maps to the persistent keys.
	checkScript := `
		(function() {
			// Re-bind shared state
			const SharedStateKeys = { sharedKey: Symbol("sharedKey") };
			// Note: We must use the EXACT same symbol description for command-specific state
			// or the exact same shared symbol for shared state.
			// For shared state, we need the canonical symbol from the registry if we want to match,
			// but here we are just checking if the fallback works.

			// Actually, for the test to work with the SAME keys as before, we need to use the
			// same persistence keys.
			// The persistence key for shared state is just the symbol name if it's registered,
			// or "command:desc" if not.

			// In the setup script:
			// const SharedStateKeys = { sharedKey: Symbol("sharedKey") };
			// const sharedState = tui.createState("__shared__", { [SharedStateKeys.sharedKey]: ... });

			// Since "sharedKey" is NOT in osm:sharedStateSymbols (unless we added it?),
			// it is treated as command-specific state for command "__shared__".
			// Key: "__shared__:sharedKey"

			const sState = tui.createState("__shared__", {
				[Symbol("sharedKey")]: {defaultValue: "defaultShared"}
			});
			const sVal = sState.get(Object.getOwnPropertySymbols(sState.get)[0] || Symbol("sharedKey"));
			// Wait, we need to pass the EXACT symbol instance to .get() that we used in definition.

			// Let's do it cleaner:
			const symShared = Symbol("sharedKey");
			const state1 = tui.createState("__shared__", { [symShared]: {defaultValue: "defaultShared"} });
			const val1 = state1.get(symShared);

			const symA = Symbol("keyA");
			const stateA = tui.createState("modeA", { [symA]: {defaultValue: 0} });
			const valA = stateA.get(symA);

			const symB = Symbol("keyB");
			const stateB = tui.createState("modeB", { [symB]: {defaultValue: "defaultB"} });
			const valB = stateB.get(symB);

			return [val1, valA, valB];
		})()
	`

	val, err := engine.vm.RunString(checkScript)
	if err != nil {
		t.Fatalf("Failed to run verification script: %v", err)
	}

	resultsObj := val.ToObject(engine.vm)
	resShared := resultsObj.Get("0").String()
	resA := resultsObj.Get("1").ToInteger()
	resB := resultsObj.Get("2").String()

	if resShared != "defaultShared" {
		t.Errorf("Expected JS shared state to be 'defaultShared', got %v", resShared)
	}
	if resA != 0 {
		t.Errorf("Expected JS modeA state to be 0, got %v", resA)
	}
	if resB != "defaultB" {
		t.Errorf("Expected JS modeB state to be 'defaultB', got %v", resB)
	}

	// Switch to modeB to check its state was also reset (already verified via JS above, but keeping flow)
	if err := tuiManager.SwitchMode("modeB"); err != nil {
		t.Fatalf("Failed to switch to modeB: %v", err)
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
