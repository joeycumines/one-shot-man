package scripting

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/joeycumines/one-shot-man/internal/storage"
)

// mockBackend implements storage.StorageBackend for testing
type mockBackend struct {
	session     *storage.Session
	loadError   error
	saveError   error
	closeError  error
	saveCalled  bool
	closeCalled bool
}

func (m *mockBackend) LoadSession(sessionID string) (*storage.Session, error) {
	if m.loadError != nil {
		return nil, m.loadError
	}
	return m.session, nil
}

func (m *mockBackend) SaveSession(session *storage.Session) error {
	m.saveCalled = true
	if m.saveError != nil {
		return m.saveError
	}
	m.session = session
	return nil
}

func (m *mockBackend) Close() error {
	m.closeCalled = true
	return m.closeError
}

func (m *mockBackend) ArchiveSession(sessionID string, destPath string) error {
	// Mock archive: no-op, just return nil
	return nil
}

func TestNewStateManager_InitializesNewSession(t *testing.T) {
	backend := &mockBackend{
		session: nil, // No existing session
	}
	sessionID := "test-session-new"

	sm, err := NewStateManager(backend, sessionID)
	if err != nil {
		t.Fatalf("NewStateManager failed: %v", err)
	}
	defer sm.Close()

	if sm == nil {
		t.Fatal("NewStateManager returned nil")
	}
	if sm.session == nil {
		t.Fatal("session was not initialized")
	}
	if sm.session.ID != sessionID {
		t.Errorf("expected ID %q, got %q", sessionID, sm.session.ID)
	}
	if sm.session.Version != "1.0.0" {
		t.Errorf("expected Version 1.0.0, got %q", sm.session.Version)
	}
	if sm.session.CreatedAt.IsZero() {
		t.Error("CreatedAt was not set")
	}
	history := sm.GetSessionHistory()
	if len(history) != 0 {
		t.Errorf("expected empty History, got %d", len(history))
	}
	if sm.session.ScriptState == nil {
		t.Error("ScriptState was not initialized")
	}
	if sm.session.SharedState == nil {
		t.Error("SharedState was not initialized")
	}
}

func TestNewStateManager_PersistsNewSession(t *testing.T) {
	backend := &mockBackend{
		session: nil,
	}
	sessionID := "test-session-persist"

	sm, err := NewStateManager(backend, sessionID)
	if err != nil {
		t.Fatalf("NewStateManager failed: %v", err)
	}
	defer sm.Close()

	if !backend.saveCalled {
		t.Error("expected backend.SaveSession to be called for new session initialization")
	}
}

func TestNewStateManager_PersistsAfterVersionMismatch(t *testing.T) {
	oldSession := &storage.Session{
		Version: "0.9.0",
		ID:      "test-session-mismatch",
	}
	backend := &mockBackend{
		session: oldSession,
	}

	sm, err := NewStateManager(backend, oldSession.ID)
	if err != nil {
		t.Fatalf("NewStateManager failed: %v", err)
	}
	defer sm.Close()

	if !backend.saveCalled {
		t.Error("expected backend.SaveSession to be called after reinitializing due to version mismatch")
	}
}

func TestNewStateManager_LoadsExistingSession(t *testing.T) {
	existingSession := &storage.Session{
		Version:     "1.0.0",
		ID:          "test-session-existing",
		CreatedAt:   time.Now().Add(-24 * time.Hour),
		UpdatedAt:   time.Now().Add(-1 * time.Hour),
		History:     []storage.HistoryEntry{{EntryID: "1", Command: "test"}},
		ScriptState: map[string]map[string]interface{}{"test-mode": {"key1": "value1"}},
		SharedState: map[string]interface{}{},
	}
	backend := &mockBackend{
		session: existingSession,
	}

	sm, err := NewStateManager(backend, existingSession.ID)
	if err != nil {
		t.Fatalf("NewStateManager failed: %v", err)
	}
	defer sm.Close()

	if sm.session != existingSession {
		t.Error("session was not loaded correctly")
	}
	history := sm.GetSessionHistory()
	if len(history) != 1 {
		t.Errorf("expected 1 history entry, got %d", len(history))
	}
}

func TestNewStateManager_RecoversFromVersionMismatch(t *testing.T) {
	oldSession := &storage.Session{
		Version:     "0.9.0", // Old version
		ID:          "test-session-old",
		CreatedAt:   time.Now().Add(-24 * time.Hour),
		UpdatedAt:   time.Now().Add(-1 * time.Hour),
		History:     []storage.HistoryEntry{{EntryID: "1", Command: "test"}},
		ScriptState: map[string]map[string]interface{}{"test-mode": {"key1": "value1"}},
		SharedState: map[string]interface{}{},
	}
	backend := &mockBackend{
		session: oldSession,
	}

	sm, err := NewStateManager(backend, oldSession.ID)
	if err != nil {
		t.Fatalf("NewStateManager failed: %v", err)
	}
	defer sm.Close()

	// Verify a fresh session was created
	if sm.session.Version != "1.0.0" {
		t.Errorf("expected Version 1.0.0, got %q", sm.session.Version)
	}
	history := sm.GetSessionHistory()
	if len(history) != 0 {
		t.Errorf("expected empty History after version mismatch, got %d", len(history))
	}
	if len(sm.session.ScriptState) != 0 {
		t.Errorf("expected empty ScriptState after version mismatch, got %d", len(sm.session.ScriptState))
	}
	if len(sm.session.SharedState) != 0 {
		t.Errorf("expected empty SharedState after version mismatch, got %d", len(sm.session.SharedState))
	}
}

// NEW ARCHITECTURE: Tests for GetState/SetState

func TestGetSetState_ScriptSpecific(t *testing.T) {
	backend := &mockBackend{
		session: nil,
	}
	sm, err := NewStateManager(backend, "test-session")
	if err != nil {
		t.Fatalf("NewStateManager failed: %v", err)
	}
	defer sm.Close()

	// Set command-specific state
	sm.SetState("test-mode:key1", "value1")
	sm.SetState("test-mode:key2", 42)

	// Get command-specific state
	val1, ok := sm.GetState("test-mode:key1")
	if !ok {
		t.Error("expected key1 to exist")
	}
	if val1 != "value1" {
		t.Errorf("expected value1, got %v", val1)
	}

	val2, ok := sm.GetState("test-mode:key2")
	if !ok {
		t.Error("expected key2 to exist")
	}
	if val2 != 42 {
		t.Errorf("expected 42, got %v", val2)
	}

	// Verify isolation between commands
	_, ok = sm.GetState("other-mode:key1")
	if ok {
		t.Error("expected key1 to not exist in other-mode")
	}
}

func TestGetSetState_Shared(t *testing.T) {
	backend := &mockBackend{
		session: nil,
	}
	sm, err := NewStateManager(backend, "test-session")
	if err != nil {
		t.Fatalf("NewStateManager failed: %v", err)
	}
	defer sm.Close()

	// Set shared state
	sm.SetState("contextItems", []string{"file1.txt", "file2.txt"})

	// Get shared state
	val, ok := sm.GetState("contextItems")
	if !ok {
		t.Error("expected contextItems to exist")
	}
	items, ok := val.([]string)
	if !ok {
		t.Fatalf("expected []string, got %T", val)
	}
	if len(items) != 2 {
		t.Errorf("expected 2 items, got %d", len(items))
	}
}

func TestCaptureSnapshot(t *testing.T) {
	backend := &mockBackend{
		session: nil,
	}
	sm, err := NewStateManager(backend, "test-session")
	if err != nil {
		t.Fatalf("NewStateManager failed: %v", err)
	}
	defer sm.Close()

	// Set some state
	sm.SetState("mode1:key1", "value1")
	sm.SetState("contextItems", "shared_value")

	// Capture a snapshot
	stateJSON, err := sm.SerializeCompleteState()
	if err != nil {
		t.Fatalf("SerializeCompleteState failed: %v", err)
	}
	err = sm.CaptureSnapshot("mode1", "test-command", stateJSON)
	if err != nil {
		t.Fatalf("CaptureSnapshot failed: %v", err)
	}

	// Verify history was updated
	history := sm.GetSessionHistory()
	if len(history) != 1 {
		t.Fatalf("expected 1 history entry, got %d", len(history))
	}
	entry := history[0]
	if entry.ModeID != "mode1" {
		t.Errorf("expected ModeID 'mode1', got %q", entry.ModeID)
	}
	if entry.Command != "test-command" {
		t.Errorf("expected Command 'test-command', got %q", entry.Command)
	}

	// Verify FinalState contains the serialized state
	if entry.FinalState == "" {
		t.Error("FinalState should not be empty")
	}
}

func TestNewStateManager_ErrorCases(t *testing.T) {
	t.Run("nil backend", func(t *testing.T) {
		_, err := NewStateManager(nil, "test-session")
		if err == nil {
			t.Error("expected error for nil backend, got nil")
		}
		if !strings.Contains(err.Error(), "backend cannot be nil") {
			t.Errorf("unexpected error message: %v", err)
		}
	})

	t.Run("empty session ID", func(t *testing.T) {
		backend := &mockBackend{}
		_, err := NewStateManager(backend, "")
		if err == nil {
			t.Error("expected error for empty sessionID, got nil")
		}
		if !strings.Contains(err.Error(), "sessionID cannot be empty") {
			t.Errorf("unexpected error message: %v", err)
		}
	})

	t.Run("backend load error", func(t *testing.T) {
		backend := &mockBackend{
			loadError: fmt.Errorf("simulated load error"),
		}
		_, err := NewStateManager(backend, "test-session")
		if err == nil {
			t.Error("expected error from backend, got nil")
		}
		if !strings.Contains(err.Error(), "failed to load session") {
			t.Errorf("unexpected error message: %v", err)
		}
	})
}

func TestPersistSession(t *testing.T) {
	backend := &mockBackend{
		session: nil,
	}
	sm, err := NewStateManager(backend, "test-session")
	if err != nil {
		t.Fatalf("NewStateManager failed: %v", err)
	}
	defer sm.Close()

	// Persist the session
	err = sm.PersistSession()
	if err != nil {
		t.Fatalf("PersistSession failed: %v", err)
	}

	if !backend.saveCalled {
		t.Error("backend.SaveSession was not called")
	}
}

func TestClose(t *testing.T) {
	backend := &mockBackend{
		session: nil,
	}
	sm, err := NewStateManager(backend, "test-session")
	if err != nil {
		t.Fatalf("NewStateManager failed: %v", err)
	}

	err = sm.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	if !backend.closeCalled {
		t.Error("backend.Close was not called")
	}
}

func TestClose_Idempotent(t *testing.T) {
	backend := &mockBackend{
		session: nil,
	}
	sm, err := NewStateManager(backend, "test-session-idempotent")
	if err != nil {
		t.Fatalf("NewStateManager failed: %v", err)
	}

	// First close should succeed
	err = sm.Close()
	if err != nil {
		t.Fatalf("First Close failed: %v", err)
	}

	if !backend.closeCalled {
		t.Error("backend.Close was not called on first close")
	}

	// Second close should also succeed (idempotent)
	err = sm.Close()
	if err != nil {
		t.Fatalf("Second Close failed: %v", err)
	}

	// Third close should also succeed
	err = sm.Close()
	if err != nil {
		t.Fatalf("Third Close failed: %v", err)
	}
}

func TestClose_Concurrent(t *testing.T) {
	backend := &mockBackend{
		session: nil,
	}
	sm, err := NewStateManager(backend, "test-session-concurrent")
	if err != nil {
		t.Fatalf("NewStateManager failed: %v", err)
	}

	// Call Close concurrently from multiple goroutines
	const numGoroutines = 10
	done := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			done <- sm.Close()
		}()
	}

	// Collect results - all should succeed
	for i := 0; i < numGoroutines; i++ {
		if err := <-done; err != nil {
			t.Errorf("Concurrent Close %d failed: %v", i, err)
		}
	}

	if !backend.closeCalled {
		t.Error("backend.Close was not called")
	}
}

// TestHistory_TruncateOnLoad verifies that large session files are
// correctly truncated on load, bounding memory usage from startup.
func TestHistory_TruncateOnLoad(t *testing.T) {
	// Setup: Create a session with maxHistoryEntries + 50 entries
	const totalEntries = maxHistoryEntries + 50
	largeHistory := make([]storage.HistoryEntry, totalEntries)
	for i := 0; i < totalEntries; i++ {
		largeHistory[i] = storage.HistoryEntry{
			EntryID:   fmt.Sprintf("entry-%d", i),
			ModeID:    "test-mode",
			Command:   fmt.Sprintf("cmd_%d", i),
			Timestamp: time.Now(),
		}
	}

	existingSession := &storage.Session{
		Version:     "1.0.0",
		ID:          "test-session-truncate",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		History:     largeHistory,
		ScriptState: make(map[string]map[string]interface{}),
		SharedState: make(map[string]interface{}),
	}

	backend := &mockBackend{
		session: existingSession,
	}

	// Action: Load the session
	sm, err := NewStateManager(backend, existingSession.ID)
	if err != nil {
		t.Fatalf("NewStateManager failed: %v", err)
	}
	defer sm.Close()

	// Assert: Check that history was truncated
	history := sm.GetSessionHistory()
	if len(history) != maxHistoryEntries {
		t.Errorf("expected history length %d, got %d", maxHistoryEntries, len(history))
	}

	// Assert: Verify the oldest entries were discarded (should start at cmd_50)
	if history[0].Command != "cmd_50" {
		t.Errorf("expected first entry to be 'cmd_50', got %q", history[0].Command)
	}

	// Assert: Verify the newest entries were kept (should end at cmd_249)
	if history[len(history)-1].Command != "cmd_249" {
		t.Errorf("expected last entry to be 'cmd_249', got %q", history[len(history)-1].Command)
	}
}

// TestHistory_RingBufferWrapAndRead verifies that the ring buffer correctly
// wraps around and maintains the most recent entries.
func TestHistory_RingBufferWrapAndRead(t *testing.T) {
	backend := &mockBackend{
		session: nil,
	}

	sm, err := NewStateManager(backend, "test-session-wrap")
	if err != nil {
		t.Fatalf("NewStateManager failed: %v", err)
	}
	defer sm.Close()

	// Action: Capture maxHistoryEntries + 10 snapshots
	const totalSnapshots = maxHistoryEntries + 10
	stateJSON := `{"script":{},"shared":{}}`
	for i := 0; i < totalSnapshots; i++ {
		command := fmt.Sprintf("cmd_%d", i)
		err = sm.CaptureSnapshot("test-mode", command, stateJSON)
		if err != nil {
			t.Fatalf("CaptureSnapshot %d failed: %v", i, err)
		}
	}

	// Assert: Check that history length is bounded to maxHistoryEntries
	history := sm.GetSessionHistory()
	if len(history) != maxHistoryEntries {
		t.Errorf("expected history length %d, got %d", maxHistoryEntries, len(history))
	}

	// Assert: Verify the oldest entries were evicted (should start at cmd_10)
	if history[0].Command != "cmd_10" {
		t.Errorf("expected first entry to be 'cmd_10', got %q", history[0].Command)
	}

	// Assert: Verify the newest entries are present (should end at cmd_209)
	if history[len(history)-1].Command != "cmd_209" {
		t.Errorf("expected last entry to be 'cmd_209', got %q", history[len(history)-1].Command)
	}
}

// TestHistory_PersistenceAfterWrap verifies that the ring buffer is correctly
// flattened when persisted to disk after wrapping around.
func TestHistory_PersistenceAfterWrap(t *testing.T) {
	backend := &mockBackend{
		session: nil,
	}

	sm, err := NewStateManager(backend, "test-session-persist-wrap")
	if err != nil {
		t.Fatalf("NewStateManager failed: %v", err)
	}
	defer sm.Close()

	// Action: Capture maxHistoryEntries + 10 snapshots
	const totalSnapshots = maxHistoryEntries + 10
	stateJSON := `{"script":{},"shared":{}}`
	for i := 0; i < totalSnapshots; i++ {
		command := fmt.Sprintf("cmd_%d", i)
		err = sm.CaptureSnapshot("test-mode", command, stateJSON)
		if err != nil {
			t.Fatalf("CaptureSnapshot %d failed: %v", i, err)
		}
	}

	// Action: Persist the session
	err = sm.PersistSession()
	if err != nil {
		t.Fatalf("PersistSession failed: %v", err)
	}

	// Assert: Check that persisted history length is bounded
	persistedHistory := backend.session.History
	if len(persistedHistory) != maxHistoryEntries {
		t.Errorf("expected persisted history length %d, got %d", maxHistoryEntries, len(persistedHistory))
	}

	// Assert: Verify chronological order (should start at cmd_10)
	if persistedHistory[0].Command != "cmd_10" {
		t.Errorf("expected first persisted entry to be 'cmd_10', got %q", persistedHistory[0].Command)
	}

	// Assert: Verify chronological order (should end at cmd_209)
	if persistedHistory[len(persistedHistory)-1].Command != "cmd_209" {
		t.Errorf("expected last persisted entry to be 'cmd_209', got %q", persistedHistory[len(persistedHistory)-1].Command)
	}
}

// TestHistory_ReadBeforeFull verifies that the ring buffer correctly returns
// history when it has not yet reached capacity.
func TestHistory_ReadBeforeFull(t *testing.T) {
	backend := &mockBackend{
		session: nil,
	}

	sm, err := NewStateManager(backend, "test-session-before-full")
	if err != nil {
		t.Fatalf("NewStateManager failed: %v", err)
	}
	defer sm.Close()

	// Action: Capture only 20 snapshots (well below maxHistoryEntries)
	const totalSnapshots = 20
	stateJSON := `{"script":{},"shared":{}}`
	for i := 0; i < totalSnapshots; i++ {
		command := fmt.Sprintf("cmd_%d", i)
		err = sm.CaptureSnapshot("test-mode", command, stateJSON)
		if err != nil {
			t.Fatalf("CaptureSnapshot %d failed: %v", i, err)
		}
	}

	// Assert: Check that history length matches the number of captures
	history := sm.GetSessionHistory()
	if len(history) != totalSnapshots {
		t.Errorf("expected history length %d, got %d", totalSnapshots, len(history))
	}

	// Assert: Verify the first entry
	if history[0].Command != "cmd_0" {
		t.Errorf("expected first entry to be 'cmd_0', got %q", history[0].Command)
	}

	// Assert: Verify the last entry
	if history[len(history)-1].Command != "cmd_19" {
		t.Errorf("expected last entry to be 'cmd_19', got %q", history[len(history)-1].Command)
	}
}

// TestHistory_PersistenceBeforeFull verifies that the ring buffer is correctly
// persisted when it has not yet reached capacity.
func TestHistory_PersistenceBeforeFull(t *testing.T) {
	backend := &mockBackend{
		session: nil,
	}

	sm, err := NewStateManager(backend, "test-session-persist-before-full")
	if err != nil {
		t.Fatalf("NewStateManager failed: %v", err)
	}
	defer sm.Close()

	// Action: Capture only 20 snapshots
	const totalSnapshots = 20
	stateJSON := `{"script":{},"shared":{}}`
	for i := 0; i < totalSnapshots; i++ {
		command := fmt.Sprintf("cmd_%d", i)
		err = sm.CaptureSnapshot("test-mode", command, stateJSON)
		if err != nil {
			t.Fatalf("CaptureSnapshot %d failed: %v", i, err)
		}
	}

	// Action: Persist the session
	err = sm.PersistSession()
	if err != nil {
		t.Fatalf("PersistSession failed: %v", err)
	}

	// Assert: Check that persisted history length matches captures
	persistedHistory := backend.session.History
	if len(persistedHistory) != totalSnapshots {
		t.Errorf("expected persisted history length %d, got %d", totalSnapshots, len(persistedHistory))
	}

	// Assert: Verify the first entry
	if persistedHistory[0].Command != "cmd_0" {
		t.Errorf("expected first persisted entry to be 'cmd_0', got %q", persistedHistory[0].Command)
	}

	// Assert: Verify the last entry
	if persistedHistory[len(persistedHistory)-1].Command != "cmd_19" {
		t.Errorf("expected last persisted entry to be 'cmd_19', got %q", persistedHistory[len(persistedHistory)-1].Command)
	}
}
