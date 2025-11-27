package scripting

import (
	"bytes"
	"context"
	"testing"
)

// TestJSStateAccessor_FallbackToDefaults verifies that the state accessor
// correctly falls back to default values when the underlying state is missing
// (e.g. after a reset).
func TestJSStateAccessor_FallbackToDefaults(t *testing.T) {
	ctx := context.Background()
	var out bytes.Buffer
	engine := mustNewEngine(t, ctx, &out, &out)

	// Define a script that creates state with defaults, modifies it, resets it (simulated), and checks values.
	setupScript := `
		const keys = {
			k1: Symbol("k1"),
			k2: Symbol("k2")
		};

		// Make keys global so we can access them in other scripts if needed,
		// but here we'll just use a closure.

		const s = tui.createState("test-cmd", {
			[keys.k1]: {defaultValue: "default1"},
			[keys.k2]: {defaultValue: 42}
		});

		tui.registerCommand({
			name: "check_defaults",
			handler: function() {
				if (s.get(keys.k1) !== "default1") throw new Error("k1 is not default: " + s.get(keys.k1));
				if (s.get(keys.k2) !== 42) throw new Error("k2 is not default: " + s.get(keys.k2));
			}
		});

		tui.registerCommand({
			name: "check_modified",
			handler: function() {
				if (s.get(keys.k1) !== "modified1") throw new Error("k1 is not modified: " + s.get(keys.k1));
				if (s.get(keys.k2) !== 100) throw new Error("k2 is not modified: " + s.get(keys.k2));
			}
		});

		tui.registerCommand({
			name: "modify_state",
			handler: function() {
				s.set(keys.k1, "modified1");
				s.set(keys.k2, 100);
			}
		});
	`

	if err := engine.ExecuteScript(engine.LoadScriptFromString("setup", setupScript)); err != nil {
		t.Fatalf("Failed to setup: %v", err)
	}

	tm := engine.GetTUIManager()

	// 1. Check initial defaults
	if err := tm.ExecuteCommand("check_defaults", nil); err != nil {
		t.Fatalf("Initial defaults check failed: %v", err)
	}

	// 2. Modify state
	if err := tm.ExecuteCommand("modify_state", nil); err != nil {
		t.Fatalf("Modify state failed: %v", err)
	}

	// 3. Check modified
	if err := tm.ExecuteCommand("check_modified", nil); err != nil {
		t.Fatalf("Modified check failed: %v", err)
	}

	// 4. Clear state (simulate reset)
	tm.stateManager.ClearAllState()

	// 5. Check defaults again (should fallback)
	if err := tm.ExecuteCommand("check_defaults", nil); err != nil {
		t.Fatalf("Fallback to defaults failed after clear: %v", err)
	}
}
