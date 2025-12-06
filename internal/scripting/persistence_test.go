package scripting

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/session"
	"github.com/joeycumines/one-shot-man/internal/storage"
)

// TestEngine_PersistenceOnClose verifies that closing the engine persists the session state.
func TestEngine_PersistenceOnClose(t *testing.T) {
	// Create a temporary directory for storage
	tmpDir := t.TempDir()
	sessionID := "test-persistence-session"
	// The session ID gets namespaced when passed as explicit override —
	// compute the effective session ID using the session package so tests
	// remain correct if the namespacing algorithm changes.
	expectedSessionID, _, err := session.GetSessionID(sessionID)
	if err != nil {
		t.Fatalf("failed to compute expected session id: %v", err)
	}

	// Override storage paths to use tmpDir
	storage.SetTestPaths(tmpDir)
	defer storage.ResetPaths()

	// Create a new engine with fs backend
	ctx := context.Background()

	// We don't need to set XDG_DATA_HOME because we used SetTestPaths.
	engine, err := NewEngineWithConfig(ctx, os.Stdout, os.Stderr, sessionID, "fs")
	if err != nil {
		t.Fatalf("Failed to create engine: %v", err)
	}

	// Set some state via TUI manager (which uses state manager)
	// We need to use the correct API: tui.createState(commandName, definitions)
	// definitions is an object where keys are Symbols and values are { defaultValue: ... }
	script := &Script{
		Name: "init",
		Content: `
			var sym = Symbol("myKey");
			var defs = {};
			defs[sym] = { defaultValue: "default" };
			var state = tui.createState("testCmd", defs);
			state.set(sym, "my-value");
		`,
	}
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("Failed to execute script: %v", err)
	}

	// Close the engine. This should trigger persistence.
	if err := engine.Close(); err != nil {
		t.Fatalf("Failed to close engine: %v", err)
	}

	// Verify that the state was persisted to disk.
	// We need to know where the file is.
	// SetTestPaths sets it to <tmpDir>/<sessionID>.session.json
	// Note: session ID is namespaced according to the session package
	expectedPath := filepath.Join(tmpDir, expectedSessionID+".session.json")

	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Fatalf("Session file was not created at %s", expectedPath)
	}

	// Load the session back to verify content
	backend, err := storage.GetBackend("fs", expectedSessionID)
	if err != nil {
		t.Fatalf("Failed to get backend: %v", err)
	}
	// Ensure we close the backend so any lock handles are released before
	// t.TempDir() cleanup runs. t.TempDir's cleanup runs after defers, and
	// backend.Close must occur before the TempDir deletion on Windows.
	defer func() {
		if err := backend.Close(); err != nil {
			t.Logf("backend.Close() failed: %v", err)
		}
	}()

	session, err := backend.LoadSession(expectedSessionID)
	if err != nil {
		t.Fatalf("Failed to load session: %v", err)
	}
	if session == nil {
		t.Fatalf("Session is nil")
	}

	// Check for the state value
	// It should be in ScriptState["testCmd"]["myKey"]
	cmdState, ok := session.ScriptState["testCmd"]
	if !ok {
		t.Fatalf("Command state 'testCmd' not found. ScriptState: %v", session.ScriptState)
	}

	val, ok := cmdState["myKey"]
	if !ok {
		t.Fatalf("State 'myKey' not found in command state. CmdState: %v", cmdState)
	}

	if val != "my-value" {
		t.Errorf("Expected state value 'my-value', got %v", val)
	}
}
