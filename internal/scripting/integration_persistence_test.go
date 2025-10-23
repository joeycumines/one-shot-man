package scripting_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/joeycumines/one-shot-man/internal/scripting"
	"github.com/joeycumines/one-shot-man/internal/scripting/storage"
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

// TestContractValidation verifies that state is correctly restored for a matching contract
// and correctly rejected for a mismatched contract.
func TestContractValidation(t *testing.T) {
	storage.ClearAllInMemorySessions()
	defer storage.ClearAllInMemorySessions()

	sessionID := "test-contract-validation"

	// Create a state manager with a contract
	backend, err := storage.NewInMemoryBackend(sessionID)
	if err != nil {
		t.Fatalf("Failed to create backend: %v", err)
	}
	defer backend.Close()

	sm, err := scripting.NewStateManager(backend, sessionID)
	if err != nil {
		t.Fatalf("Failed to create state manager: %v", err)
	}
	defer sm.Close()

	// Register a contract
	contract1 := storage.ContractDefinition{
		ModeName: "test-mode",
		IsShared: false,
		Keys: map[string]any{
			"counter": 0,
			"name":    "default",
		},
		Schemas: map[string]any{
			"counter": "number",
			"name":    "string",
		},
	}

	if err := sm.RegisterContract(contract1); err != nil {
		t.Fatalf("Failed to register contract: %v", err)
	}

	// Save some state
	stateJSON := `{"counter":42,"name":"test"}`
	stateMap := map[string]string{
		"test-mode": stateJSON,
	}

	if err := sm.CaptureSnapshot("test-mode", "test command", stateMap); err != nil {
		t.Fatalf("Failed to capture snapshot: %v", err)
	}

	if err := sm.PersistSession(); err != nil {
		t.Fatalf("Failed to persist session: %v", err)
	}

	// Test 1: Restore with matching contract
	backend2, err := storage.NewInMemoryBackend(sessionID)
	if err != nil {
		t.Fatalf("Failed to create second backend: %v", err)
	}
	defer backend2.Close()

	sm2, err := scripting.NewStateManager(backend2, sessionID)
	if err != nil {
		t.Fatalf("Failed to create second state manager: %v", err)
	}
	defer sm2.Close()

	if err := sm2.RegisterContract(contract1); err != nil {
		t.Fatalf("Failed to register matching contract: %v", err)
	}

	restoredState, err := sm2.RestoreState("test-mode", false)
	if err != nil {
		t.Fatalf("Failed to restore state: %v", err)
	}

	if restoredState != stateJSON {
		t.Errorf("State mismatch. Expected: %s, Got: %s", stateJSON, restoredState)
	}

	// Test 2: Try to restore with mismatched contract (different key)
	contract2 := storage.ContractDefinition{
		ModeName: "test-mode",
		IsShared: false,
		Keys: map[string]any{
			"counter":  0,
			"newField": "added", // Changed contract
		},
		Schemas: map[string]any{
			"counter":  "number",
			"newField": "string",
		},
	}

	backend3, err := storage.NewInMemoryBackend(sessionID)
	if err != nil {
		t.Fatalf("Failed to create third backend: %v", err)
	}
	defer backend3.Close()

	sm3, err := scripting.NewStateManager(backend3, sessionID)
	if err != nil {
		t.Fatalf("Failed to create third state manager: %v", err)
	}
	defer sm3.Close()

	if err := sm3.RegisterContract(contract2); err != nil {
		t.Fatalf("Failed to register mismatched contract: %v", err)
	}

	mismatchedState, err := sm3.RestoreState("test-mode", false)
	if err != nil {
		t.Fatalf("Unexpected error on contract mismatch: %v", err)
	}

	// Should return empty string when contract doesn't match
	if mismatchedState != "" {
		t.Errorf("Expected empty state on contract mismatch, got: %s", mismatchedState)
	}
}

// TestEndToEndLifecycle is a full integration test that starts the TUI, runs several commands
// in different modes, exits, re-initializes a new TUI with the same session, and verifies
// that the state is correctly restored.
func TestEndToEndLifecycle(t *testing.T) {
	storage.ClearAllInMemorySessions()
	defer storage.ClearAllInMemorySessions()

	// Use unique session ID to avoid conflicts when tests run in parallel
	sessionID := fmt.Sprintf("test-end-to-end-%d", time.Now().UnixNano())

	// Create a simple test script that defines a mode with state
	testScript := `
		const contract = tui.createStateContract('test-mode', {
			counter: {
				description: 'test-mode:counter',
				defaultValue: 0
			},
			name: {
				description: 'test-mode:name',
				defaultValue: 'initial'
			}
		});

		tui.registerMode({
			name: 'test-mode',
			stateContract: contract,
			tui: {
				enableHistory: true
			},
			commands: function(state) {
				return {
					increment: {
						description: 'Increment counter',
						handler: function(args) {
							const current = state.get(contract.counter) || 0;
							state.set(contract.counter, current + 1);
						}
					},
					setname: {
						description: 'Set name',
						handler: function(args) {
							if (args.length > 0) {
								state.set(contract.name, args[0]);
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
	if counterVal2 != int64(2) {
		t.Errorf("Expected restored counter=2, got %v", counterVal2)
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
		tmpDir := os.Getenv("TEST_STALE_LOCK_DIR")
		sessionID := "test-stale-lock"

		storage.SetTestPaths(tmpDir)

		_, err := storage.NewFileSystemBackend(sessionID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to create backend: %v\n", err)
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

	// Launch subprocess
	cmd := exec.Command(os.Args[0], "-test.run=TestStaleLockRecovery")
	cmd.Env = append(os.Environ(),
		"TEST_STALE_LOCK_SUBPROCESS=1",
		"TEST_STALE_LOCK_DIR="+tmpDir,
	)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("Failed to create stdout pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start subprocess: %v", err)
	}

	// Wait for subprocess to acquire lock with timeout
	done := make(chan bool)
	var lockAcquired bool

	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			if strings.Contains(scanner.Text(), "LOCK_ACQUIRED") {
				lockAcquired = true
				done <- true
				return
			}
		}
		done <- false
	}()

	select {
	case <-done:
		if !lockAcquired {
			cmd.Process.Kill()
			cmd.Wait()
			t.Fatal("Subprocess did not acquire lock")
		}
	case <-time.After(3 * time.Second):
		cmd.Process.Kill()
		cmd.Wait()
		t.Fatal("Timeout waiting for subprocess to acquire lock")
	}

	// Kill the subprocess to simulate crash
	if err := cmd.Process.Signal(syscall.SIGKILL); err != nil {
		t.Fatalf("Failed to kill subprocess: %v", err)
	}

	cmd.Wait() // Ignore error, we killed it

	// Now try to acquire the lock - should succeed immediately
	sessionID := "test-stale-lock"
	backend, err := storage.NewFileSystemBackend(sessionID)
	if err != nil {
		t.Fatalf("Failed to acquire lock after subprocess crash: %v", err)
	}
	defer backend.Close()
}

// TestSharedStateRoundtrip verifies that shared state set in one mode is correctly
// persisted and is available after restarting the session, both in the original mode
// and a different mode.
func TestSharedStateRoundtrip(t *testing.T) {
	storage.ClearAllInMemorySessions()
	defer storage.ClearAllInMemorySessions()

	sessionID := "test-shared-roundtrip"

	// Create test script with shared state and two modes
	testScript := `
		const sharedContract = tui.createSharedStateContract('shared-data', {
			counter: {
				description: '__shared__:counter',
				defaultValue: 0
			},
			message: {
				description: '__shared__:message',
				defaultValue: 'none'
			}
		});

		tui.registerMode({
			name: 'mode-a',
			stateContract: sharedContract,
			tui: { enableHistory: true },
			commands: function(state) {
				return {
					increment: {
						description: 'Increment shared counter',
						handler: function(args) {
							const current = state.get(sharedContract.counter) || 0;
							state.set(sharedContract.counter, current + 1);
						}
					},
					setmsg: {
						description: 'Set shared message',
						handler: function(args) {
							if (args.length > 0) {
								state.set(sharedContract.message, args[0]);
							}
						}
					}
				};
			}
		});

		tui.registerMode({
			name: 'mode-b',
			stateContract: sharedContract,
			tui: { enableHistory: true },
			commands: function(state) {
				return {
					verify: {
						description: 'Verify shared state',
						handler: function(args) {
							// Just access the state
						}
					}
				};
			}
		});
	`

	// Session 1: Set shared state from mode-a
	ctx := context.Background()
	engine1, err := scripting.NewEngineWithConfig(ctx, io.Discard, io.Discard, sessionID, "memory")
	if err != nil {
		t.Fatalf("Failed to create first engine: %v", err)
	}
	defer engine1.Close()

	script1 := engine1.LoadScriptFromString("test-shared.js", testScript)
	if err := engine1.ExecuteScript(script1); err != nil {
		t.Fatalf("Failed to execute test script: %v", err)
	}

	tm1 := engine1.GetTUIManager()
	if tm1 == nil {
		t.Fatal("TUI manager is nil")
	}

	// Switch to mode-a and modify shared state
	if err := tm1.SwitchMode("mode-a"); err != nil {
		t.Fatalf("Failed to switch to mode-a: %v", err)
	}

	if err := tm1.ExecuteCommand("increment", []string{}); err != nil {
		t.Fatalf("Failed to execute increment: %v", err)
	}
	if err := tm1.ExecuteCommand("increment", []string{}); err != nil {
		t.Fatalf("Failed to execute increment: %v", err)
	}
	if err := tm1.ExecuteCommand("increment", []string{}); err != nil {
		t.Fatalf("Failed to execute increment: %v", err)
	}
	if err := tm1.ExecuteCommand("setmsg", []string{"shared-test"}); err != nil {
		t.Fatalf("Failed to execute setmsg: %v", err)
	}

	// Verify state before persist
	counterVal, err := tm1.GetStateForTest("__shared__:counter")
	if err != nil {
		t.Fatalf("Failed to get counter state: %v", err)
	}
	if counterVal != int64(3) {
		t.Errorf("Expected counter=3, got %v", counterVal)
	}

	messageVal, err := tm1.GetStateForTest("__shared__:message")
	if err != nil {
		t.Fatalf("Failed to get message state: %v", err)
	}
	if messageVal != "shared-test" {
		t.Errorf("Expected message='shared-test', got %v", messageVal)
	}

	// Persist the session
	if err := tm1.PersistSessionForTest(); err != nil {
		t.Fatalf("Failed to persist session: %v", err)
	}

	if err := tm1.Close(); err != nil {
		t.Fatalf("Failed to close first TUI manager: %v", err)
	}

	// Session 2: Create new engine with same session ID, load in mode-b
	engine2, err := scripting.NewEngineWithConfig(ctx, io.Discard, io.Discard, sessionID, "memory")
	if err != nil {
		t.Fatalf("Failed to create second engine: %v", err)
	}
	defer engine2.Close()

	script2 := engine2.LoadScriptFromString("test-shared.js", testScript)
	if err := engine2.ExecuteScript(script2); err != nil {
		t.Fatalf("Failed to execute test script in second session: %v", err)
	}

	tm2 := engine2.GetTUIManager()
	if tm2 == nil {
		t.Fatal("Second TUI manager is nil")
	}

	// Switch to mode-b (different from mode-a)
	if err := tm2.SwitchMode("mode-b"); err != nil {
		t.Fatalf("Failed to switch to mode-b in second session: %v", err)
	}

	// Verify shared state was restored and is accessible from mode-b
	counterVal2, err := tm2.GetStateForTest("__shared__:counter")
	if err != nil {
		t.Fatalf("Failed to get counter state in second session: %v", err)
	}
	if counterVal2 != int64(3) {
		t.Errorf("Expected restored counter=3, got %v", counterVal2)
	}

	messageVal2, err := tm2.GetStateForTest("__shared__:message")
	if err != nil {
		t.Fatalf("Failed to get message state in second session: %v", err)
	}
	if messageVal2 != "shared-test" {
		t.Errorf("Expected restored message='shared-test', got %v", messageVal2)
	}

	// Also verify we can switch back to mode-a and still access the shared state
	if err := tm2.SwitchMode("mode-a"); err != nil {
		t.Fatalf("Failed to switch back to mode-a: %v", err)
	}

	counterVal3, err := tm2.GetStateForTest("__shared__:counter")
	if err != nil {
		t.Fatalf("Failed to get counter state in mode-a: %v", err)
	}
	if counterVal3 != int64(3) {
		t.Errorf("Expected counter=3 in mode-a, got %v", counterVal3)
	}

	if err := tm2.Close(); err != nil {
		t.Fatalf("Failed to close second TUI manager: %v", err)
	}
}

// TestSigintPersistence verifies that state is persisted when a command executes with history enabled.
// This test uses the direct TUI manager API to verify the behavior that would occur on SIGINT.
func TestSigintPersistence(t *testing.T) {
	// Set up test environment with file system backend
	sessionID := "test-sigint"
	tmpDir := t.TempDir()

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
		const contract = tui.createStateContract('sigint-mode', {
			value: {
				description: 'sigint-mode:value',
				defaultValue: 'unset'
			}
		});

		tui.registerMode({
			name: 'sigint-mode',
			stateContract: contract,
			tui: { enableHistory: true },
			commands: function(state) {
				return {
					setvalue: {
						description: 'Set value',
						handler: function(args) {
							if (args.length > 0) {
								state.set(contract.value, args[0]);
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
	sessionPath, err := storage.SessionFilePath(sessionID)
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

	if session.SessionID != sessionID {
		t.Errorf("Session ID mismatch: expected %s, got %s", sessionID, session.SessionID)
	}

	// Verify the state was saved
	if len(session.LatestState) == 0 {
		t.Fatal("No state saved in session")
	}

	// The state should contain the sigint-mode state
	modeState, ok := session.LatestState["sigint-mode"]
	if !ok {
		t.Fatal("sigint-mode state not found in session")
	}

	// Verify the value was persisted
	if !strings.Contains(modeState.StateJSON, "test-from-sigint") {
		t.Errorf("Expected state to contain 'test-from-sigint', got: %s", modeState.StateJSON)
	}
}
