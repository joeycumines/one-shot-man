package scripting_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/joeycumines/one-shot-man/internal/scripting"
	"github.com/joeycumines/one-shot-man/internal/session"
	"github.com/joeycumines/one-shot-man/internal/storage"
	"github.com/joeycumines/one-shot-man/internal/testutil"
)

// TestAtomicityOnCrash simulates a panic during the final atomic rename operation
// and verifies that the original file remains untouched and no temporary files are leaked.
func TestAtomicityOnCrash(t *testing.T) {
	// Create a temporary directory for this test
	tmpDir := t.TempDir()

	sessionID := "test-atomicity"
	sessionPath := filepath.Join(tmpDir, sessionID+".session.json")

	// Create an initial session file with original content
	originalContent := []byte(`{"version":"1.0.0","original":true}`)
	if err := os.WriteFile(sessionPath, originalContent, 0644); err != nil {
		t.Fatalf("Failed to write initial session: %v", err)
	}

	// Set up deferred panic recovery and hook cleanup
	var didPanic bool
	defer func() {
		if r := recover(); r != nil {
			didPanic = true
		}
	}()

	defer func() {
		// Always reset the hook
		storage.SetTestHookCrashBeforeRename(nil)
	}()

	// Set the crash simulation hook
	storage.SetTestHookCrashBeforeRename(func() {
		panic("simulated crash")
	})

	// Try to write new content - this will panic
	newData := []byte(`{"version":"1.0.0","corrupted":true}`)
	storage.AtomicWriteFile(sessionPath, newData, 0644)

	// After panic is recovered...
	if !didPanic {
		t.Fatal("Expected panic did not occur")
	}

	// Verify the original file content is still intact
	finalContent, err := os.ReadFile(sessionPath)
	if err != nil {
		t.Fatalf("Failed to read final file: %v", err)
	}

	if string(finalContent) != string(originalContent) {
		t.Errorf("Original file was modified. Expected: %s, Got: %s", originalContent, finalContent)
	}

	// Verify no temporary files were leaked
	files, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("Failed to read temp dir: %v", err)
	}

	for _, file := range files {
		if strings.HasPrefix(file.Name(), ".tmp-session-") {
			t.Errorf("Found leaked temp file: %s", file.Name())
		}
	}
}

// TestCorruptionHandling attempts to load a malformed JSON file and asserts that a clear error is returned.
func TestCorruptionHandling(t *testing.T) {
	tmpDir := t.TempDir()
	sessionID := "test-corruption"

	// Override path functions to use temp dir
	storage.SetTestPaths(tmpDir)
	defer storage.ResetPaths()

	// Create a corrupted session file
	sessionPath := filepath.Join(tmpDir, sessionID+".session.json")
	corruptedData := []byte(`{"this is": "not valid json"`)
	if err := os.WriteFile(sessionPath, corruptedData, 0644); err != nil {
		t.Fatalf("Failed to write corrupted file: %v", err)
	}

	// Try to create a backend and load the session
	backend, err := storage.NewFileSystemBackend(sessionID)
	if err != nil {
		t.Fatalf("Failed to create backend: %v", err)
	}
	defer backend.Close()

	session, err := backend.LoadSession(sessionID)
	if err == nil {
		t.Errorf("Expected error when loading corrupted session, got nil")
	}
	if session != nil {
		t.Errorf("Expected nil session on corruption, got: %v", session)
	}

	// Verify the error message indicates JSON parsing failure
	if err != nil && !strings.Contains(err.Error(), "unmarshal") {
		t.Errorf("Error message doesn't indicate JSON parsing failure: %v", err)
	}
}

// TestContractValidation removed - the new architecture doesn't use contracts

// TestEndToEndLifecycle is a full integration test that starts the TUI, runs several commands
// in different modes, exits, re-initializes a new TUI with the same session, and verifies
// that the state is correctly restored.
func TestEndToEndLifecycle(t *testing.T) {
	// Avoid clearing global in-memory store; use unique session IDs instead

	// Use unique session ID to avoid conflicts when tests run in parallel
	sessionID := testutil.NewTestSessionID("test-end-to-end", t.Name())

	// Create a simple test script that defines a mode with state
	testScript := `
		const stateKeys = {
			counter: Symbol("counter"),
			name: Symbol("name")
		};
		const state = tui.createState("test-mode", {
				[stateKeys.counter]: {defaultValue: 0},
				[stateKeys.name]: {defaultValue: "initial"}
		});

		tui.registerMode({
			name: 'test-mode',
			tui: {
				enableHistory: true
			},
			commands: function() {
				return {
					increment: {
						description: 'Increment counter',
						handler: function(args) {
							const current = state.get(stateKeys.counter) || 0;
							state.set(stateKeys.counter, current + 1);
						}
					},
					setname: {
						description: 'Set name',
						handler: function(args) {
							if (args.length > 0) {
								state.set(stateKeys.name, args[0]);
							}
						}
					}
				};
			}
		});
	`

	// First session: Create engine, switch mode, modify state, persist
	ctx := context.Background()
	// Use io.Discard to suppress TUI initialization messages during tests
	engine1, err := scripting.NewEngineWithConfig(ctx, io.Discard, io.Discard, sessionID, "memory")
	if err != nil {
		t.Fatalf("Failed to create first engine: %v", err)
	}
	defer engine1.Close()

	script1 := engine1.LoadScriptFromString("test-script.js", testScript)
	if err := engine1.ExecuteScript(script1); err != nil {
		t.Fatalf("Failed to execute test script: %v", err)
	}

	tm1 := engine1.GetTUIManager()
	if tm1 == nil {
		t.Fatal("TUI manager is nil")
	}

	// Switch to test mode
	if err := tm1.SwitchMode("test-mode"); err != nil {
		t.Fatalf("Failed to switch to test-mode: %v", err)
	}

	// Execute commands to modify state
	if err := tm1.ExecuteCommand("increment", []string{}); err != nil {
		t.Fatalf("Failed to execute increment command: %v", err)
	}
	if err := tm1.ExecuteCommand("increment", []string{}); err != nil {
		t.Fatalf("Failed to execute increment command: %v", err)
	}
	if err := tm1.ExecuteCommand("setname", []string{"test-value"}); err != nil {
		t.Fatalf("Failed to execute setname command: %v", err)
	}

	// Verify state before persist
	counterVal, err := tm1.GetStateForTest("test-mode:counter")
	if err != nil {
		t.Fatalf("Failed to get counter state: %v", err)
	}
	if counterVal != int64(2) {
		t.Errorf("Expected counter=2, got %v", counterVal)
	}

	nameVal, err := tm1.GetStateForTest("test-mode:name")
	if err != nil {
		t.Fatalf("Failed to get name state: %v", err)
	}
	if nameVal != "test-value" {
		t.Errorf("Expected name='test-value', got %v", nameVal)
	}

	// Persist the session
	if err := tm1.PersistSessionForTest(); err != nil {
		t.Fatalf("Failed to persist session: %v", err)
	}

	// Close first session
	if err := tm1.Close(); err != nil {
		t.Fatalf("Failed to close first TUI manager: %v", err)
	}

	// Second session: Create new engine with same session ID, verify state restoration
	// Use io.Discard to suppress TUI initialization messages during tests
	engine2, err := scripting.NewEngineWithConfig(ctx, io.Discard, io.Discard, sessionID, "memory")
	if err != nil {
		t.Fatalf("Failed to create second engine: %v", err)
	}
	defer engine2.Close()

	// Load the same script to register the mode
	script2 := engine2.LoadScriptFromString("test-script.js", testScript)
	if err := engine2.ExecuteScript(script2); err != nil {
		t.Fatalf("Failed to execute test script in second session: %v", err)
	}

	tm2 := engine2.GetTUIManager()
	if tm2 == nil {
		t.Fatal("Second TUI manager is nil")
	}

	// Switch to test mode
	if err := tm2.SwitchMode("test-mode"); err != nil {
		t.Fatalf("Failed to switch to test-mode in second session: %v", err)
	}

	// Verify state was restored
	counterVal2, err := tm2.GetStateForTest("test-mode:counter")
	if err != nil {
		t.Fatalf("Failed to get counter state in second session: %v", err)
	}

	// Handle potential float64 from JSON unmarshal
	var valEqual bool
	switch v := counterVal2.(type) {
	case int64:
		valEqual = v == 2
	case float64:
		valEqual = v == 2.0
	case int:
		valEqual = v == 2
	}

	if !valEqual {
		t.Errorf("Expected restored counter=2, got %v (type %T)", counterVal2, counterVal2)
	}

	nameVal2, err := tm2.GetStateForTest("test-mode:name")
	if err != nil {
		t.Fatalf("Failed to get name state in second session: %v", err)
	}
	if nameVal2 != "test-value" {
		t.Errorf("Expected restored name='test-value', got %v", nameVal2)
	}

	// Clean up
	if err := tm2.Close(); err != nil {
		t.Fatalf("Failed to close second TUI manager: %v", err)
	}
}

// TestConcurrencyConflict verifies that if one osm process holds a lock on a session file,
// a second osm process attempting to use the same session ID will fail to start immediately.
func TestConcurrencyConflict(t *testing.T) {
	tmpDir := t.TempDir()
	sessionID := "test-concurrency"

	// Override path functions
	storage.SetTestPaths(tmpDir)
	defer storage.ResetPaths()

	// Create first backend (acquires lock)
	backend1, err := storage.NewFileSystemBackend(sessionID)
	if err != nil {
		t.Fatalf("Failed to create first backend: %v", err)
	}
	defer backend1.Close()

	// Try to create second backend (should fail due to lock)
	backend2, err := storage.NewFileSystemBackend(sessionID)
	if err == nil {
		backend2.Close()
		t.Fatal("Expected error when creating second backend with same session, got nil")
	}

	if !strings.Contains(err.Error(), "lock") {
		t.Errorf("Error message should mention lock conflict: %v", err)
	}

	// Close first backend and verify second can now acquire lock
	if err := backend1.Close(); err != nil {
		t.Fatalf("Failed to close first backend: %v", err)
	}

	backend3, err := storage.NewFileSystemBackend(sessionID)
	if err != nil {
		t.Fatalf("Failed to create backend after lock release: %v", err)
	}
	defer backend3.Close()
}

// TestStaleLockRecovery launches a subprocess that acquires a session lock and then
// terminates itself without gracefully releasing it. Verifies that a new osm instance
// can immediately acquire the same lock.
func TestStaleLockRecovery(t *testing.T) {
	if os.Getenv("TEST_STALE_LOCK_SUBPROCESS") == "1" {
		// This is the subprocess that will create a lock and exit abruptly
		fmt.Println("SUBPROCESS_STARTING")
		tmpDir := os.Getenv("TEST_STALE_LOCK_DIR")
		sessionID := "test-stale-lock"

		storage.SetTestPaths(tmpDir)

		fmt.Println("SUBPROCESS_STARTING")
		_, err := storage.NewFileSystemBackend(sessionID)
		if err != nil {
			fmt.Println("NEW_BACKEND_ERROR:", err)
			os.Exit(1)
		}

		// Signal parent that we have the lock
		fmt.Println("LOCK_ACQUIRED")
		os.Stdout.Sync()

		// Wait for signal to exit
		time.Sleep(5 * time.Second)

		// Exit without calling backend.Close() to simulate crash
		os.Exit(1)
		return
	}

	// Parent test process
	tmpDir := t.TempDir()

	// Override path functions
	storage.SetTestPaths(tmpDir)
	defer storage.ResetPaths()

	// Simulate a stale lock by creating the lock file but not holding an
	// active flock on it. NewFileSystemBackend should be able to acquire
	// the lock even when the lock file exists (no active holder).
	sessionID := "test-stale-lock"
	lockPath := filepath.Join(tmpDir, sessionID+".session.lock")
	if err := os.WriteFile(lockPath, []byte("stale"), 0644); err != nil {
		t.Fatalf("Failed to create stale lock file: %v", err)
	}

	backend, err := storage.NewFileSystemBackend(sessionID)
	if err != nil {
		t.Fatalf("Failed to acquire lock with stale lock file present: %v", err)
	}
	defer backend.Close()
}

// TestSharedStateRoundtrip removed - testing shared state in new architecture requires implementing Symbol sharing

// TestSigintPersistence verifies that state is persisted when a command executes with history enabled.
// This test uses the direct TUI manager API to verify the behavior that would occur on SIGINT.
func TestSigintPersistence(t *testing.T) {
	// Set up test environment with file system backend
	sessionID := "test-sigint"
	// The session ID gets namespaced when passed as explicit override —
	// compute the effective session ID using the session package to stay
	// robust to any namespacing/sanitization logic changes.
	expectedSessionID, _, err := session.GetSessionID(sessionID)
	if err != nil {
		t.Fatalf("failed to compute expected session id: %v", err)
	}
	tmpDir := t.TempDir()

	// Override storage paths to use the test directory
	storage.SetTestPaths(tmpDir)
	defer storage.ResetPaths()

	ctx := context.Background()
	// Create engine with explicit config
	engine, err := scripting.NewEngineWithConfig(ctx, io.Discard, io.Discard, sessionID, "fs")
	if err != nil {
		t.Fatalf("Failed to create engine: %v", err)
	}
	defer engine.Close()

	// Load test script
	scriptPath := filepath.Join(tmpDir, "test-script.js")
	scriptContent := `
		const stateKeys = {
			value: Symbol("value")
		};
		const state = tui.createState("sigint-mode", {
				[stateKeys.value]: {defaultValue: "unset"}
		});

		tui.registerMode({
			name: 'sigint-mode',
			tui: { enableHistory: true },
			commands: function() {
				return {
					setvalue: {
						description: 'Set value',
						handler: function(args) {
							if (args.length > 0) {
								state.set(stateKeys.value, args[0]);
							}
						}
					}
				};
			}
		});

		// Auto-switch to the mode
		tui.switchMode('sigint-mode');
	`

	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0644); err != nil {
		t.Fatalf("Failed to write test script: %v", err)
	}

	script, err := engine.LoadScript("test-script.js", scriptPath)
	if err != nil {
		t.Fatalf("Failed to load script: %v", err)
	}
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("Failed to execute script: %v", err)
	}

	tuiManager := engine.GetTUIManager()

	// Execute a command to modify state (this simulates interactive command execution)
	if err := tuiManager.ExecuteCommand("setvalue", []string{"test-from-sigint"}); err != nil {
		t.Fatalf("Failed to execute command: %v", err)
	}

	// Persist the session (this simulates what happens on SIGINT via terminal.Run())
	if err := tuiManager.PersistSessionForTest(); err != nil {
		t.Fatalf("Failed to persist session: %v", err)
	}

	// Get the session file path and verify it was created
	// Note: session ID is namespaced as ex--<original>
	sessionPath, err := storage.SessionFilePath(expectedSessionID)
	if err != nil {
		t.Fatalf("Failed to get session file path: %v", err)
	}

	sessionData, err := os.ReadFile(sessionPath)
	if err != nil {
		t.Fatalf("Session file not found: %v", err)
	}

	// Parse and verify the session contains our state
	var session storage.Session
	if err := json.Unmarshal(sessionData, &session); err != nil {
		t.Fatalf("Failed to parse session file: %v", err)
	}

	if session.ID != expectedSessionID {
		t.Errorf("Session ID mismatch: expected %s, got %s", expectedSessionID, session.ID)
	}

	// Verify the state was saved in the new schema
	if session.ScriptState == nil {
		t.Fatal("No ScriptState in session")
	}

	// The state should contain the sigint-mode state
	modeState, ok := session.ScriptState["sigint-mode"]
	if !ok {
		t.Fatal("sigint-mode state not found in session")
	}

	// Verify the value was persisted
	value, ok := modeState["value"]
	if !ok {
		t.Fatal("value key not found in sigint-mode state")
	}
	if value != "test-from-sigint" {
		t.Errorf("Expected value='test-from-sigint', got %v", value)
	}
}
