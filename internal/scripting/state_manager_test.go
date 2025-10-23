package scripting

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/joeycumines/one-shot-man/internal/scripting/storage"
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
	if sm.session.SessionID != sessionID {
		t.Errorf("expected SessionID %q, got %q", sessionID, sm.session.SessionID)
	}
	if sm.session.Version != "1.0.0" {
		t.Errorf("expected Version 1.0.0, got %q", sm.session.Version)
	}
	if sm.session.CreatedAt.IsZero() {
		t.Error("CreatedAt was not set")
	}
	if len(sm.session.Contracts) != 0 {
		t.Errorf("expected empty Contracts, got %d", len(sm.session.Contracts))
	}
	history := sm.GetSessionHistory()
	if len(history) != 0 {
		t.Errorf("expected empty History, got %d", len(history))
	}
	if sm.session.LatestState == nil {
		t.Error("LatestState was not initialized")
	}
}

func TestNewStateManager_LoadsExistingSession(t *testing.T) {
	existingSession := &storage.Session{
		Version:   "1.0.0",
		SessionID: "test-session-existing",
		CreatedAt: time.Now().Add(-24 * time.Hour),
		UpdatedAt: time.Now().Add(-1 * time.Hour),
		Contracts: []storage.ContractDefinition{
			{ModeName: "test-mode", Keys: map[string]any{"key1": "value1"}},
		},
		History: []storage.HistoryEntry{
			{EntryID: "1", Command: "test"},
		},
		LatestState: map[string]storage.ModeState{
			"test-mode": {ModeName: "test-mode", ContractHash: "abc123", StateJSON: `{"key1":"value1"}`},
		},
	}
	backend := &mockBackend{
		session: existingSession,
	}

	sm, err := NewStateManager(backend, existingSession.SessionID)
	if err != nil {
		t.Fatalf("NewStateManager failed: %v", err)
	}
	defer sm.Close()

	if sm.session != existingSession {
		t.Error("session was not loaded correctly")
	}
	if len(sm.session.Contracts) != 1 {
		t.Errorf("expected 1 contract, got %d", len(sm.session.Contracts))
	}
	history := sm.GetSessionHistory()
	if len(history) != 1 {
		t.Errorf("expected 1 history entry, got %d", len(history))
	}
}

func TestNewStateManager_RecoversFromVersionMismatch(t *testing.T) {
	oldSession := &storage.Session{
		Version:   "0.9.0", // Old version
		SessionID: "test-session-old",
		CreatedAt: time.Now().Add(-24 * time.Hour),
		UpdatedAt: time.Now().Add(-1 * time.Hour),
		Contracts: []storage.ContractDefinition{
			{ModeName: "test-mode", Keys: map[string]any{"key1": "value1"}},
		},
		History: []storage.HistoryEntry{
			{EntryID: "1", Command: "test"},
		},
		LatestState: map[string]storage.ModeState{
			"test-mode": {ModeName: "test-mode", ContractHash: "abc123", StateJSON: `{"key1":"value1"}`},
		},
	}
	backend := &mockBackend{
		session: oldSession,
	}

	sm, err := NewStateManager(backend, oldSession.SessionID)
	if err != nil {
		t.Fatalf("NewStateManager failed: %v", err)
	}
	defer sm.Close()

	// Verify a fresh session was created
	if sm.session.Version != "1.0.0" {
		t.Errorf("expected Version 1.0.0, got %q", sm.session.Version)
	}
	if len(sm.session.Contracts) != 0 {
		t.Errorf("expected empty Contracts after version mismatch, got %d", len(sm.session.Contracts))
	}
	history := sm.GetSessionHistory()
	if len(history) != 0 {
		t.Errorf("expected empty History after version mismatch, got %d", len(history))
	}
	if len(sm.session.LatestState) != 0 {
		t.Errorf("expected empty LatestState after version mismatch, got %d", len(sm.session.LatestState))
	}
}

func TestRestoreState_Success(t *testing.T) {
	backend := &mockBackend{
		session: nil,
	}
	sm, err := NewStateManager(backend, "test-session")
	if err != nil {
		t.Fatalf("NewStateManager failed: %v", err)
	}
	defer sm.Close()

	// Register a contract
	contract := storage.ContractDefinition{
		ModeName: "test-mode",
		IsShared: false,
		Keys:     map[string]any{"key1": "default1", "key2": 42},
		Schemas:  map[string]any{"key1": "string", "key2": "number"},
	}
	err = sm.RegisterContract(contract)
	if err != nil {
		t.Fatalf("RegisterContract failed: %v", err)
	}

	// Compute the expected hash
	expectedHash, err := storage.ComputeContractSchemaHash(contract)
	if err != nil {
		t.Fatalf("ComputeContractSchemaHash failed: %v", err)
	}

	// Add a matching state to the session
	stateJSON := `{"key1":"test-value","key2":99}`
	sm.session.LatestState["test-mode"] = storage.ModeState{
		ModeName:     "test-mode",
		ContractHash: expectedHash,
		StateJSON:    stateJSON,
	}

	// Restore the state
	restored, err := sm.RestoreState("test-mode", false)
	if err != nil {
		t.Fatalf("RestoreState failed: %v", err)
	}
	if restored != stateJSON {
		t.Errorf("expected state %q, got %q", stateJSON, restored)
	}
}

func TestRestoreState_HashMismatch(t *testing.T) {
	backend := &mockBackend{
		session: nil,
	}
	sm, err := NewStateManager(backend, "test-session")
	if err != nil {
		t.Fatalf("NewStateManager failed: %v", err)
	}
	defer sm.Close()

	// Register a contract
	contract := storage.ContractDefinition{
		ModeName: "test-mode",
		IsShared: false,
		Keys:     map[string]any{"key1": "default1"},
		Schemas:  map[string]any{"key1": "string"},
	}
	err = sm.RegisterContract(contract)
	if err != nil {
		t.Fatalf("RegisterContract failed: %v", err)
	}

	// Add a state with a MISMATCHED hash
	stateJSON := `{"key1":"test-value"}`
	sm.session.LatestState["test-mode"] = storage.ModeState{
		ModeName:     "test-mode",
		ContractHash: "wrong-hash-12345",
		StateJSON:    stateJSON,
	}

	// Restore the state - should return empty string due to hash mismatch
	restored, err := sm.RestoreState("test-mode", false)
	if err != nil {
		t.Fatalf("RestoreState failed: %v", err)
	}
	if restored != "" {
		t.Errorf("expected empty string for mismatched hash, got %q", restored)
	}
}

func TestRestoreState_NoContractRegistered(t *testing.T) {
	backend := &mockBackend{
		session: nil,
	}
	sm, err := NewStateManager(backend, "test-session")
	if err != nil {
		t.Fatalf("NewStateManager failed: %v", err)
	}
	defer sm.Close()

	// Try to restore state for a mode without registering a contract
	_, err = sm.RestoreState("unregistered-mode", false)
	if err == nil {
		t.Error("expected error for unregistered contract, got nil")
	}
	if !strings.Contains(err.Error(), "no contract registered") {
		t.Errorf("expected 'no contract registered' error, got: %v", err)
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

	// Register contracts
	contract1 := storage.ContractDefinition{
		ModeName: "mode1",
		IsShared: false,
		Keys:     map[string]any{"key1": "default1"},
		Schemas:  map[string]any{"key1": "string"},
	}
	contract2 := storage.ContractDefinition{
		ModeName: "shared",
		IsShared: true,
		Keys:     map[string]any{"shared_key": "default"},
		Schemas:  map[string]any{"shared_key": "string"},
	}
	sm.RegisterContract(contract1)
	sm.RegisterContract(contract2)

	// Capture a snapshot
	stateMap := map[string]string{
		"mode1":      `{"key1":"value1"}`,
		"__shared__": `{"shared_key":"shared_value"}`,
	}
	err = sm.CaptureSnapshot("mode1", "test-command", stateMap)
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

	// Verify FinalState contains the state map
	var finalState map[string]string
	err = json.Unmarshal([]byte(entry.FinalState), &finalState)
	if err != nil {
		t.Fatalf("failed to unmarshal FinalState: %v", err)
	}
	if finalState["mode1"] != stateMap["mode1"] {
		t.Errorf("expected mode1 state %q, got %q", stateMap["mode1"], finalState["mode1"])
	}
	if finalState["__shared__"] != stateMap["__shared__"] {
		t.Errorf("expected __shared__ state %q, got %q", stateMap["__shared__"], finalState["__shared__"])
	}

	// Verify LatestState was updated
	if len(sm.session.LatestState) != 2 {
		t.Fatalf("expected 2 LatestState entries, got %d", len(sm.session.LatestState))
	}
	if sm.session.LatestState["mode1"].StateJSON != stateMap["mode1"] {
		t.Errorf("expected LatestState mode1 %q, got %q", stateMap["mode1"], sm.session.LatestState["mode1"].StateJSON)
	}
	if sm.session.LatestState["__shared__"].StateJSON != stateMap["__shared__"] {
		t.Errorf("expected LatestState __shared__ %q, got %q", stateMap["__shared__"], sm.session.LatestState["__shared__"].StateJSON)
	}

	// Verify contract hashes are stored
	if len(entry.ContractHashes) != 2 {
		t.Fatalf("expected 2 contract hashes, got %d", len(entry.ContractHashes))
	}
}

func TestRegisterContract_UpdatesAndReplaces(t *testing.T) {
	backend := &mockBackend{
		session: nil,
	}
	sm, err := NewStateManager(backend, "test-session")
	if err != nil {
		t.Fatalf("NewStateManager failed: %v", err)
	}
	defer sm.Close()

	// Register initial contract
	contract1 := storage.ContractDefinition{
		ModeName: "test-mode",
		IsShared: false,
		Keys:     map[string]any{"key1": "default1"},
		Schemas:  map[string]any{"key1": "string"},
	}
	err = sm.RegisterContract(contract1)
	if err != nil {
		t.Fatalf("RegisterContract failed: %v", err)
	}

	if len(sm.session.Contracts) != 1 {
		t.Fatalf("expected 1 contract, got %d", len(sm.session.Contracts))
	}
	if sm.session.Contracts[0].ModeName != "test-mode" {
		t.Errorf("expected ModeName 'test-mode', got %q", sm.session.Contracts[0].ModeName)
	}
	if len(sm.registeredContracts) != 1 {
		t.Fatalf("expected 1 registered contract, got %d", len(sm.registeredContracts))
	}

	// Register a different contract for the same mode (should replace)
	contract2 := storage.ContractDefinition{
		ModeName: "test-mode",
		IsShared: false,
		Keys:     map[string]any{"key1": "default1", "key2": 42},
		Schemas:  map[string]any{"key1": "string", "key2": "number"},
	}
	err = sm.RegisterContract(contract2)
	if err != nil {
		t.Fatalf("RegisterContract failed: %v", err)
	}

	// Should still have only 1 contract (the new one)
	if len(sm.session.Contracts) != 1 {
		t.Fatalf("expected 1 contract after replacement, got %d", len(sm.session.Contracts))
	}
	// Verify it's the new contract
	if len(sm.session.Contracts[0].Keys) != 2 {
		t.Errorf("expected 2 keys in replaced contract, got %d", len(sm.session.Contracts[0].Keys))
	}

	// Register a contract for a different mode
	contract3 := storage.ContractDefinition{
		ModeName: "another-mode",
		IsShared: false,
		Keys:     map[string]any{"other_key": "value"},
	}
	err = sm.RegisterContract(contract3)
	if err != nil {
		t.Fatalf("RegisterContract failed: %v", err)
	}

	// Should now have 2 contracts
	if len(sm.session.Contracts) != 2 {
		t.Fatalf("expected 2 contracts, got %d", len(sm.session.Contracts))
	}
	if len(sm.registeredContracts) != 2 {
		t.Fatalf("expected 2 registered contracts, got %d", len(sm.registeredContracts))
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
		SessionID:   "test-session-truncate",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		Contracts:   []storage.ContractDefinition{},
		History:     largeHistory,
		LatestState: make(map[string]storage.ModeState),
	}

	backend := &mockBackend{
		session: existingSession,
	}

	// Action: Load the session
	sm, err := NewStateManager(backend, existingSession.SessionID)
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

	// Register a test contract
	contract := storage.ContractDefinition{
		ModeName: "test-mode",
		IsShared: false,
		Keys:     map[string]any{"key1": "value1"},
		Schemas:  map[string]any{"key1": "string"},
	}
	err = sm.RegisterContract(contract)
	if err != nil {
		t.Fatalf("RegisterContract failed: %v", err)
	}

	// Action: Capture maxHistoryEntries + 10 snapshots
	const totalSnapshots = maxHistoryEntries + 10
	stateMap := map[string]string{"test-mode": `{"key1":"value1"}`}
	for i := 0; i < totalSnapshots; i++ {
		command := fmt.Sprintf("cmd_%d", i)
		err = sm.CaptureSnapshot("test-mode", command, stateMap)
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

	// Register a test contract
	contract := storage.ContractDefinition{
		ModeName: "test-mode",
		IsShared: false,
		Keys:     map[string]any{"key1": "value1"},
		Schemas:  map[string]any{"key1": "string"},
	}
	err = sm.RegisterContract(contract)
	if err != nil {
		t.Fatalf("RegisterContract failed: %v", err)
	}

	// Action: Capture maxHistoryEntries + 10 snapshots
	const totalSnapshots = maxHistoryEntries + 10
	stateMap := map[string]string{"test-mode": `{"key1":"value1"}`}
	for i := 0; i < totalSnapshots; i++ {
		command := fmt.Sprintf("cmd_%d", i)
		err = sm.CaptureSnapshot("test-mode", command, stateMap)
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

	// Register a test contract
	contract := storage.ContractDefinition{
		ModeName: "test-mode",
		IsShared: false,
		Keys:     map[string]any{"key1": "value1"},
		Schemas:  map[string]any{"key1": "string"},
	}
	err = sm.RegisterContract(contract)
	if err != nil {
		t.Fatalf("RegisterContract failed: %v", err)
	}

	// Action: Capture only 20 snapshots (well below maxHistoryEntries)
	const totalSnapshots = 20
	stateMap := map[string]string{"test-mode": `{"key1":"value1"}`}
	for i := 0; i < totalSnapshots; i++ {
		command := fmt.Sprintf("cmd_%d", i)
		err = sm.CaptureSnapshot("test-mode", command, stateMap)
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

	// Register a test contract
	contract := storage.ContractDefinition{
		ModeName: "test-mode",
		IsShared: false,
		Keys:     map[string]any{"key1": "value1"},
		Schemas:  map[string]any{"key1": "string"},
	}
	err = sm.RegisterContract(contract)
	if err != nil {
		t.Fatalf("RegisterContract failed: %v", err)
	}

	// Action: Capture only 20 snapshots
	const totalSnapshots = 20
	stateMap := map[string]string{"test-mode": `{"key1":"value1"}`}
	for i := 0; i < totalSnapshots; i++ {
		command := fmt.Sprintf("cmd_%d", i)
		err = sm.CaptureSnapshot("test-mode", command, stateMap)
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
