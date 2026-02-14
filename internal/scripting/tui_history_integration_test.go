package scripting

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/joeycumines/one-shot-man/internal/storage"
	"github.com/joeycumines/one-shot-man/internal/testutil"
)

// TestExtractCommandHistory tests the extractCommandHistory helper function
func TestExtractCommandHistory(t *testing.T) {
	t.Run("empty history", func(t *testing.T) {
		entries := []storage.HistoryEntry{}
		result := extractCommandHistory(entries)
		if len(result) != 0 {
			t.Errorf("expected empty slice, got %d entries", len(result))
		}
	})

	t.Run("single entry", func(t *testing.T) {
		entries := []storage.HistoryEntry{
			{
				EntryID:  "1",
				Command:  "help",
				ReadTime: time.Now(),
			},
		}
		result := extractCommandHistory(entries)
		if len(result) != 1 {
			t.Fatalf("expected 1 entry, got %d", len(result))
		}
		if result[0] != "help" {
			t.Errorf("expected command 'help', got '%s'", result[0])
		}
	})

	t.Run("multiple entries", func(t *testing.T) {
		entries := []storage.HistoryEntry{
			{EntryID: "1", Command: "help", ReadTime: time.Now()},
			{EntryID: "2", Command: "list modes", ReadTime: time.Now()},
			{EntryID: "3", Command: "switch demo", ReadTime: time.Now()},
			{EntryID: "4", Command: "exit", ReadTime: time.Now()},
		}
		result := extractCommandHistory(entries)
		if len(result) != 4 {
			t.Fatalf("expected 4 entries, got %d", len(result))
		}

		expectedCommands := []string{"help", "list modes", "switch demo", "exit"}
		for i, cmd := range expectedCommands {
			if result[i] != cmd {
				t.Errorf("entry %d: expected command '%s', got '%s'", i, cmd, result[i])
			}
		}
	})

	t.Run("preserves order", func(t *testing.T) {
		entries := []storage.HistoryEntry{
			{EntryID: "1", Command: "first", ReadTime: time.Now()},
			{EntryID: "2", Command: "second", ReadTime: time.Now()},
			{EntryID: "3", Command: "third", ReadTime: time.Now()},
		}
		result := extractCommandHistory(entries)

		if result[0] != "first" || result[1] != "second" || result[2] != "third" {
			t.Errorf("order not preserved: got %v", result)
		}
	})

	t.Run("handles commands with special characters", func(t *testing.T) {
		entries := []storage.HistoryEntry{
			{EntryID: "1", Command: "echo \"hello world\"", ReadTime: time.Now()},
			{EntryID: "2", Command: "cmd --flag=value", ReadTime: time.Now()},
			{EntryID: "3", Command: "path/to/file.txt", ReadTime: time.Now()},
		}
		result := extractCommandHistory(entries)

		if result[0] != "echo \"hello world\"" {
			t.Errorf("expected 'echo \"hello world\"', got '%s'", result[0])
		}
		if result[1] != "cmd --flag=value" {
			t.Errorf("expected 'cmd --flag=value', got '%s'", result[1])
		}
		if result[2] != "path/to/file.txt" {
			t.Errorf("expected 'path/to/file.txt', got '%s'", result[2])
		}
	})
}

// TestNewTUIManager_CommandHistoryInitialization tests that NewTUIManager properly initializes commandHistory.
// This tests the actual NewTUIManager constructor, not just the helper function.
func TestNewTUIManager_CommandHistoryInitialization(t *testing.T) {
	t.Run("with state manager and session history", func(t *testing.T) {
		ctx := context.Background()
		engine := mustNewEngine(t, ctx, io.Discard, io.Discard)

		// Create TUI manager through the engine
		tm := engine.GetTUIManager()
		if tm == nil {
			t.Fatal("TUI manager was not created")
		}

		// Verify commandHistory is initialized (not nil)
		if tm.commandHistory == nil {
			t.Error("commandHistory should be initialized, got nil")
		}

		// The actual content depends on state manager initialization success/failure,
		// but the field should always be non-nil
		t.Logf("commandHistory initialized with %d entries", len(tm.commandHistory))
	})
}

// TestNewTUIManager_StateManagerErrorPath tests that NewTUIManager handles StateManager initialization errors gracefully.
// This tests the actual error path in NewTUIManager when initializeStateManager fails.
func TestNewTUIManager_StateManagerErrorPath(t *testing.T) {
	t.Run("continues with empty commandHistory when state manager fails", func(t *testing.T) {
		ctx := context.Background()
		var output strings.Builder
		engine := mustNewEngine(t, ctx, &output, &output)

		// Create TUI manager with invalid storage backend to trigger error path
		tm := NewTUIManagerWithConfig(ctx, engine, io.NopCloser(strings.NewReader("")), &output, "", "invalid-backend-that-does-not-exist")
		if tm == nil {
			t.Fatal("NewTUIManager should not return nil even on state manager failure")
		}

		// Verify warning message was printed
		outputStr := output.String()
		if !strings.Contains(outputStr, "Warning: Failed to initialize state persistence") {
			t.Errorf("Expected warning about state persistence failure, got: %s", outputStr)
		}
		// The manager should have fallen back to memory-backed state; don't
		// assert on a specific "ephemeral mode" string as backends may emit
		// varying diagnostic text in different environments.

		// Verify commandHistory is initialized (empty)
		if tm.commandHistory == nil {
			t.Error("commandHistory should be initialized even on state manager failure")
		}
		if len(tm.commandHistory) != 0 {
			t.Errorf("commandHistory should be empty when state manager fails, got %d entries", len(tm.commandHistory))
		}

		// Verify stateManager is NOT nil (it should be memory backend)
		if tm.stateManager == nil {
			t.Error("stateManager should not be nil (should fallback to memory)")
		}
	})
}

// TestNewTUIManager_NilSessionPath tests that NewTUIManager handles the case where
// StateManager is created successfully but session is nil.
func TestNewTUIManager_NilSessionPath(t *testing.T) {
	t.Run("initializes empty commandHistory when session is nil", func(t *testing.T) {
		// Create a backend that returns nil session (simulating a corrupt/empty backend)
		backend := &mockBackend{
			session: nil, // This will cause NewStateManager to create a new session, never nil
		}

		sm, err := NewStateManager(backend, "test-nil-session")
		if err != nil {
			t.Fatalf("NewStateManager failed: %v", err)
		}
		defer sm.Close()

		// Note: NewStateManager actually initializes a new session if backend returns nil,
		// so this path is actually testing that we handle an empty History correctly
		if sm.session == nil {
			t.Fatal("NewStateManager initialized session, cannot test nil path directly")
		}

		// Test through extractCommandHistory instead, which is what NewTUIManager uses
		commandHistory := extractCommandHistory(sm.GetSessionHistory())

		if commandHistory == nil {
			t.Error("extractCommandHistory should never return nil")
		}
		if len(commandHistory) != 0 {
			t.Errorf("expected empty commandHistory, got %d entries", len(commandHistory))
		}
	})
}

// TestNewTUIManager_LoadsHistoryFromSession tests that NewTUIManager properly loads
// command history when StateManager provides a session with existing history.
// This is a TRUE integration test that calls NewTUIManager.
func TestNewTUIManager_LoadsHistoryFromSession(t *testing.T) {
	t.Run("loads existing history through full NewTUIManager flow", func(t *testing.T) {
		// Note: This test can't easily inject a mock backend into NewTUIManager since
		// initializeStateManager is called internally. We test the golden path here
		// and the error paths separately.

		ctx := context.Background()
		var output strings.Builder
		engine := mustNewEngine(t, ctx, &output, &output)

		// Use a unique session ID for this test
		testSessionID := testutil.NewTestSessionID("test-history-load", t.Name())

		// Create first TUI manager to establish a session (using fs backend)
		tm1 := NewTUIManagerWithConfig(ctx, engine, io.NopCloser(strings.NewReader("")), &output, testSessionID, "memory")
		if tm1 == nil {
			t.Fatal("first NewTUIManager failed")
		}
		if tm1.stateManager == nil {
			t.Fatal("stateManager should be initialized")
		}

		// Add some history entries via CaptureSnapshot
		err := tm1.stateManager.CaptureSnapshot("test-mode", "test command 1", json.RawMessage(`{"script":{},"shared":{}}`))
		if err != nil {
			t.Fatalf("failed to capture snapshot 1: %v", err)
		}
		err = tm1.stateManager.CaptureSnapshot("test-mode", "test command 2", json.RawMessage(`{"script":{},"shared":{}}`))
		if err != nil {
			t.Fatalf("failed to capture snapshot 2: %v", err)
		}
		if err := tm1.stateManager.PersistSession(); err != nil {
			t.Fatalf("failed to persist session: %v", err)
		}
		tm1.Close()

		// Create second TUI manager with same session ID - should load history
		output.Reset()
		tm2 := NewTUIManagerWithConfig(ctx, engine, io.NopCloser(strings.NewReader("")), &output, testSessionID, "memory")
		if tm2 == nil {
			t.Fatal("second NewTUIManager failed")
		}
		defer tm2.Close()

		// Verify history was loaded
		if len(tm2.commandHistory) != 2 {
			t.Fatalf("expected 2 history entries, got %d", len(tm2.commandHistory))
		}
		if tm2.commandHistory[0] != "test command 1" {
			t.Errorf("expected 'test command 1', got '%s'", tm2.commandHistory[0])
		}
		if tm2.commandHistory[1] != "test command 2" {
			t.Errorf("expected 'test command 2', got '%s'", tm2.commandHistory[1])
		}
	})
}

// TestCommandHistory_PreservesChronologicalOrder tests that history maintains chronological order
func TestCommandHistory_PreservesChronologicalOrder(t *testing.T) {
	now := time.Now()
	entries := []storage.HistoryEntry{
		{EntryID: "1", Command: "first", ReadTime: now.Add(-3 * time.Hour)},
		{EntryID: "2", Command: "second", ReadTime: now.Add(-2 * time.Hour)},
		{EntryID: "3", Command: "third", ReadTime: now.Add(-1 * time.Hour)},
		{EntryID: "4", Command: "fourth", ReadTime: now},
	}

	result := extractCommandHistory(entries)

	// The order should be preserved as-is (chronological, oldest to newest)
	expectedOrder := []string{"first", "second", "third", "fourth"}

	if len(result) != len(expectedOrder) {
		t.Fatalf("expected %d entries, got %d", len(expectedOrder), len(result))
	}

	for i, expected := range expectedOrder {
		if result[i] != expected {
			t.Errorf("position %d: expected '%s', got '%s'", i, expected, result[i])
		}
	}
}

// TestCommandHistory_LargeHistorySet tests extractCommandHistory with a large number of entries
func TestCommandHistory_LargeHistorySet(t *testing.T) {
	const numEntries = 1000
	entries := make([]storage.HistoryEntry, numEntries)

	for i := 0; i < numEntries; i++ {
		entries[i] = storage.HistoryEntry{
			EntryID:  string(rune(i)),
			Command:  "command " + string(rune(i)),
			ReadTime: time.Now().Add(time.Duration(i) * time.Second),
		}
	}

	result := extractCommandHistory(entries)

	if len(result) != numEntries {
		t.Fatalf("expected %d commands, got %d", numEntries, len(result))
	}

	// Spot check a few entries
	if result[0] != "command \x00" {
		t.Errorf("first entry incorrect: got '%s'", result[0])
	}

	if result[numEntries-1] != "command "+string(rune(numEntries-1)) {
		t.Errorf("last entry incorrect: got '%s'", result[numEntries-1])
	}
}

// TestNewTUIManager_StateManagerIntegration tests the full integration with StateManager
// through the actual NewTUIManager constructor, not just helper functions.
func TestNewTUIManager_StateManagerIntegration(t *testing.T) {
	t.Run("full lifecycle with persistence", func(t *testing.T) {
		ctx := context.Background()
		var output strings.Builder
		engine := mustNewEngine(t, ctx, &output, &output)

		// Use unique session ID
		testSessionID := testutil.NewTestSessionID("test-lifecycle", t.Name())

		// Step 1: Create first TUI manager - should initialize new session
		tm1 := NewTUIManagerWithConfig(ctx, engine, io.NopCloser(strings.NewReader("")), &output, testSessionID, "memory")
		if tm1 == nil {
			t.Fatal("first NewTUIManager failed")
		}
		if tm1.stateManager == nil {
			t.Fatal("stateManager should be initialized")
		}
		if len(tm1.commandHistory) != 0 {
			t.Errorf("expected empty initial history, got %d entries", len(tm1.commandHistory))
		}

		// Step 2: Add history and persist
		err := tm1.stateManager.CaptureSnapshot("test-mode", "initial command", json.RawMessage(`{"script":{},"shared":{}}`))
		if err != nil {
			t.Fatalf("failed to capture snapshot: %v", err)
		}
		if err := tm1.stateManager.PersistSession(); err != nil {
			t.Fatalf("failed to persist session: %v", err)
		}
		tm1.Close()

		// Step 3: Create second TUI manager - should load persisted history
		output.Reset()
		tm2 := NewTUIManagerWithConfig(ctx, engine, io.NopCloser(strings.NewReader("")), &output, testSessionID, "memory")
		if tm2 == nil {
			t.Fatal("second NewTUIManager failed")
		}
		defer tm2.Close()

		if len(tm2.commandHistory) != 1 {
			t.Fatalf("expected 1 loaded command, got %d", len(tm2.commandHistory))
		}
		if tm2.commandHistory[0] != "initial command" {
			t.Errorf("expected 'initial command', got '%s'", tm2.commandHistory[0])
		}

		// Step 4: Add more history
		err = tm2.stateManager.CaptureSnapshot("test-mode", "new command", json.RawMessage(`{"script":{},"shared":{}}`))
		if err != nil {
			t.Fatalf("failed to capture snapshot: %v", err)
		}
		if err := tm2.stateManager.PersistSession(); err != nil {
			t.Fatalf("failed to persist updated session: %v", err)
		}

		// Step 5: Verify history was updated
		// Reload to confirm persistence
		tm2.Close()
		output.Reset()
		tm3 := NewTUIManagerWithConfig(ctx, engine, io.NopCloser(strings.NewReader("")), &output, testSessionID, "memory")
		if tm3 == nil {
			t.Fatal("third NewTUIManager failed")
		}
		defer tm3.Close()

		if len(tm3.commandHistory) != 2 {
			t.Fatalf("expected 2 commands after reload, got %d", len(tm3.commandHistory))
		}
		if tm3.commandHistory[1] != "new command" {
			t.Errorf("expected 'new command', got '%s'", tm3.commandHistory[1])
		}
	})
}

// TestCommandHistory_EmptyAndNilSafety tests edge cases with nil and empty values
func TestCommandHistory_EmptyAndNilSafety(t *testing.T) {
	t.Run("nil history slice", func(t *testing.T) {
		// This simulates the case where we have a nil history slice
		// extractCommandHistory should handle this gracefully
		var nilHistory []storage.HistoryEntry

		result := extractCommandHistory(nilHistory)

		if result == nil {
			t.Error("result should not be nil, expected empty slice")
		}

		if len(result) != 0 {
			t.Errorf("expected empty result, got %d entries", len(result))
		}
	})

	t.Run("empty command strings", func(t *testing.T) {
		entries := []storage.HistoryEntry{
			{EntryID: "1", Command: "", ReadTime: time.Now()},
			{EntryID: "2", Command: "valid", ReadTime: time.Now()},
			{EntryID: "3", Command: "", ReadTime: time.Now()},
		}

		result := extractCommandHistory(entries)

		if len(result) != 3 {
			t.Fatalf("expected 3 entries, got %d", len(result))
		}

		// Empty commands should be preserved as-is
		if result[0] != "" {
			t.Errorf("expected empty string, got '%s'", result[0])
		}
		if result[1] != "valid" {
			t.Errorf("expected 'valid', got '%s'", result[1])
		}
		if result[2] != "" {
			t.Errorf("expected empty string, got '%s'", result[2])
		}
	})
}
