package scripting

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/dop251/goja"
	"github.com/joeycumines/one-shot-man/internal/scripting/storage"
)

const maxHistoryEntries = 200

// archiveAttemptsMax controls how many candidate archive filenames ArchiveAndReset
// will try before giving up. Tests may override this value for simulation.

// StateManager orchestrates all persistence logic for the TUI.
type StateManager struct {
	mu        sync.Mutex // Protects session and history buffer
	backend   storage.StorageBackend
	sessionID string
	session   *storage.Session

	// Symbol maps for shared state identification
	sharedSymbolToString map[goja.Value]string
	sharedStringToSymbol map[string]goja.Value
	sessionMu            sync.RWMutex // Protects symbol maps

	// History ring buffer
	historyBuf   []storage.HistoryEntry // Fixed-size physical buffer (size == maxHistoryEntries)
	historyStart int                    // Index of the oldest entry in historyBuf
	historyLen   int                    // Number of valid, populated entries in historyBuf
	// ArchiveAttemptsMax controls how many candidate archive filenames ArchiveAndReset
	// will try before giving up. Zero means use the built-in default.
	ArchiveAttemptsMax int
}

// NewStateManager creates a new state manager and loads or initializes the session.
func NewStateManager(backend storage.StorageBackend, sessionID string) (*StateManager, error) {
	if backend == nil {
		return nil, fmt.Errorf("backend cannot be nil")
	}
	if sessionID == "" {
		return nil, fmt.Errorf("sessionID cannot be empty")
	}

	// Try to load existing session
	session, err := backend.LoadSession(sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to load session: %w", err)
	}

	// Initialize a new session if one doesn't exist
	isNewSession := session == nil
	reinitialized := false
	if isNewSession {
		session = &storage.Session{
			Version:     "1.0.0",
			SessionID:   sessionID,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
			History:     []storage.HistoryEntry{},
			ScriptState: make(map[string]map[string]interface{}),
			SharedState: make(map[string]interface{}),
		}
	} else {
		// Handle schema migration if needed
		if session.Version != "1.0.0" {
			log.Printf("WARNING: Session schema version mismatch. Expected 1.0.0, got %s. Starting fresh session.", session.Version)
			session = &storage.Session{
				Version:     "1.0.0",
				SessionID:   sessionID,
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
				History:     []storage.HistoryEntry{},
				ScriptState: make(map[string]map[string]interface{}),
				SharedState: make(map[string]interface{}),
			}
			reinitialized = true
		}

		// Ensure ScriptState and SharedState are initialized
		if session.ScriptState == nil {
			session.ScriptState = make(map[string]map[string]interface{})
		}
		if session.SharedState == nil {
			session.SharedState = make(map[string]interface{})
		}
	}

	sm := &StateManager{
		backend:              backend,
		sessionID:            sessionID,
		session:              session,
		sharedSymbolToString: make(map[goja.Value]string),
		sharedStringToSymbol: make(map[string]goja.Value),
		// Initialize the fixed-size physical buffer
		historyBuf:   make([]storage.HistoryEntry, maxHistoryEntries),
		historyStart: 0,
		historyLen:   0,
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Get the history slice loaded from disk
	loadedHistory := sm.session.History

	// Immediately nil the session reference to allow GC
	sm.session.History = nil

	// Calculate truncation
	startIndex := 0
	if len(loadedHistory) > maxHistoryEntries {
		startIndex = len(loadedHistory) - maxHistoryEntries
	}

	if len(loadedHistory) > 0 {

		// Copy the truncated data into our new buffer
		n := copy(sm.historyBuf, loadedHistory[startIndex:])

		// Set the logical length
		sm.historyLen = n
	}

	// If this is a brand new session or we reinitialized due to a version
	// mismatch, persist it immediately so it becomes discoverable by other
	// processes (e.g., `osm session list`) while this session is active.
	// Perform this irrespective of loadedHistory length (including empty
	// histories) so freshly-created sessions are written.
	if isNewSession || reinitialized {
		if err := sm.persistSessionInternal(); err != nil {
			return nil, fmt.Errorf("failed to persist new session: %w", err)
		}
	}

	return sm, nil
}

// getFlatHistoryInternal reconstructs the chronological history from
// the ring buffer into a new, flat slice.
// It ASSUMES that sm.mu is already held by the caller.
func (sm *StateManager) getFlatHistoryInternal() []storage.HistoryEntry {
	if sm.historyLen == 0 {
		return nil
	}

	// Create a new slice with the exact capacity needed
	flatHistory := make([]storage.HistoryEntry, 0, sm.historyLen)

	// Check if the buffer data wraps around
	endIndex := sm.historyStart + sm.historyLen

	if endIndex <= maxHistoryEntries {
		// No wrap-around: data is in a single contiguous block
		flatHistory = append(flatHistory, sm.historyBuf[sm.historyStart:endIndex]...)
	} else {
		// Wrap-around: data is in two blocks

		// 1. Part 1: from historyStart to the end of the buffer
		flatHistory = append(flatHistory, sm.historyBuf[sm.historyStart:maxHistoryEntries]...)

		// 2. Part 2: from the beginning of the buffer to the end
		endIndexWrap := endIndex % maxHistoryEntries
		flatHistory = append(flatHistory, sm.historyBuf[0:endIndexWrap]...)
	}

	return flatHistory
}

// SetSharedSymbols is called by the shared_symbols module loader.
func (sm *StateManager) SetSharedSymbols(symbolToString map[goja.Value]string, stringToSymbol map[string]goja.Value) {
	sm.sessionMu.Lock()
	defer sm.sessionMu.Unlock()
	sm.sharedSymbolToString = symbolToString
	sm.sharedStringToSymbol = stringToSymbol
}

// IsSharedSymbol checks if a goja.Value Symbol is a known shared symbol.
// If true, it returns its canonical string key (e.g., "contextItems").
func (sm *StateManager) IsSharedSymbol(symbol goja.Value) (string, bool) {
	sm.sessionMu.RLock()
	defer sm.sessionMu.RUnlock()
	key, ok := sm.sharedSymbolToString[symbol]
	return key, ok
}

// GetState retrieves a value from the unified state map.
// Key format: "commandID:localKey" for command-specific, or shared symbol name for shared state.
func (sm *StateManager) GetState(persistentKey string) (interface{}, bool) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Ensure maps are initialized
	if sm.session.ScriptState == nil {
		sm.session.ScriptState = make(map[string]map[string]interface{})
	}
	if sm.session.SharedState == nil {
		sm.session.SharedState = make(map[string]interface{})
	}

	// Check if this is a shared symbol (no colon prefix)
	if !strings.Contains(persistentKey, ":") {
		// Shared state lookup
		val, ok := sm.session.SharedState[persistentKey]
		return val, ok
	}

	// Script-specific state: split "commandID:localKey"
	parts := strings.SplitN(persistentKey, ":", 2)
	if len(parts) != 2 {
		return nil, false
	}
	commandID := parts[0]
	localKey := parts[1]

	// Get command's state map
	commandState, exists := sm.session.ScriptState[commandID]
	if !exists {
		return nil, false
	}

	val, ok := commandState[localKey]
	return val, ok
}

// SetState sets a value in the unified state map.
// Key format: "commandID:localKey" for command-specific, or shared symbol name for shared state.
func (sm *StateManager) SetState(persistentKey string, value interface{}) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Ensure maps are initialized
	if sm.session.ScriptState == nil {
		sm.session.ScriptState = make(map[string]map[string]interface{})
	}
	if sm.session.SharedState == nil {
		sm.session.SharedState = make(map[string]interface{})
	}

	// Check if this is a shared symbol (no colon prefix)
	if !strings.Contains(persistentKey, ":") {
		// Shared state write
		sm.session.SharedState[persistentKey] = value
		sm.session.UpdatedAt = time.Now()
		return
	}

	// Script-specific state: split "commandID:localKey"
	parts := strings.SplitN(persistentKey, ":", 2)
	if len(parts) != 2 {
		return
	}
	commandID := parts[0]
	localKey := parts[1]

	// Get or create command's state map
	commandState, exists := sm.session.ScriptState[commandID]
	if !exists {
		commandState = make(map[string]interface{})
		sm.session.ScriptState[commandID] = commandState
	}

	commandState[localKey] = value
	sm.session.UpdatedAt = time.Now()
}

// SerializeCompleteState serializes the entire state (both script and shared) into a JSON string.
// This is used by CaptureSnapshot to create history entries with complete state snapshots.
func (sm *StateManager) SerializeCompleteState() (string, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Ensure maps are initialized
	if sm.session.ScriptState == nil {
		sm.session.ScriptState = make(map[string]map[string]interface{})
	}
	if sm.session.SharedState == nil {
		sm.session.SharedState = make(map[string]interface{})
	}

	// Create a wrapper object containing both state zones
	completeState := map[string]interface{}{
		"script": sm.session.ScriptState,
		"shared": sm.session.SharedState,
	}

	bytes, err := json.Marshal(completeState)
	if err != nil {
		return "{}", fmt.Errorf("failed to serialize state: %w", err)
	}

	return string(bytes), nil
}

// CaptureSnapshot captures the current state into the session history.
// This creates an immutable history entry with the complete application state.
func (sm *StateManager) CaptureSnapshot(modeID, command string, stateJSON string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Generate a unique entry ID
	entryID := fmt.Sprintf("%d", time.Now().UnixNano())

	// Create the history entry
	entry := storage.HistoryEntry{
		EntryID:    entryID,
		ModeID:     modeID,
		Command:    command,
		Timestamp:  time.Now(),
		FinalState: stateJSON,
	}

	// Ring buffer write logic
	// 1. Calculate the next available slot index
	nextIndex := (sm.historyStart + sm.historyLen) % maxHistoryEntries

	// 2. Insert the new entry into the physical buffer
	sm.historyBuf[nextIndex] = entry

	// 3. Update logical pointers
	if sm.historyLen < maxHistoryEntries {
		// Buffer is not full, just increase length
		sm.historyLen++
	} else {
		// Buffer is full, "pop" the oldest entry by advancing the start
		sm.historyStart = (sm.historyStart + 1) % maxHistoryEntries
	}

	sm.session.UpdatedAt = time.Now()

	return nil
}

// persistSessionInternal saves the session to the backend.
// It ASSUMES that sm.mu is already held by the caller.
func (sm *StateManager) persistSessionInternal() error {
	// Serialize the ring buffer back into sm.session.History
	// so it can be written to disk by the backend.
	sm.session.History = sm.getFlatHistoryInternal()
	if sm.backend == nil {
		// Nothing to persist to
		return nil
	}
	if err := sm.backend.SaveSession(sm.session); err != nil {
		return fmt.Errorf("failed to persist session: %w", err)
	}
	return nil
}

// PersistSession writes the entire session to disk via the storage backend.
func (sm *StateManager) PersistSession() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	return sm.persistSessionInternal()
}

// GetSessionHistory safely retrieves a copy of the command history from the session.
// The returned slice is a new allocation and is safe for the caller to modify.
func (sm *StateManager) GetSessionHistory() []storage.HistoryEntry {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	return sm.getFlatHistoryInternal()
}

// ClearAllState resets all script and shared state to empty maps.
// It does NOT clear the history.
func (sm *StateManager) ClearAllState() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.session.ScriptState = make(map[string]map[string]interface{})
	sm.session.SharedState = make(map[string]interface{})
	sm.session.UpdatedAt = time.Now()
}

// ArchiveAndReset performs a safe reset that archives the current session and reinitializes.
// This is called by the reset REPL command to preserve history while clearing state.
// Steps:
//  1. Persist current session to ensure all in-memory state is on disk
//  2. Archive the session file to {archive}/{sanitizedID}--reset--{timestamp}--{counter}.session.json
//     (counter increments if file already exists, allowing multiple resets with same timestamp)
//  3. Clear all state and reinitialize to defaults
//  4. Persist the new empty session under the original session ID filename
//
// Returns the archive path and any error encountered.
func (sm *StateManager) ArchiveAndReset() (string, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.backend == nil {
		return "", fmt.Errorf("backend is nil, cannot archive")
	}

	// Step 1: Persist current state to disk
	if err := sm.persistSessionInternal(); err != nil {
		return "", fmt.Errorf("failed to persist session before archive: %w", err)
	}

	// Step 2: Determine archive path and archive the session file
	// Use a counter to handle collisions when multiple resets happen in quick succession
	ts := time.Now()
	counter := 0
	archivePath := ""

	// Try candidate archive paths until we successfully create one without
	// overwriting an existing file. ArchiveSession will return os.ErrExist
	// if the destination already exists which avoids a TOCTOU window.
	// Determine the effective attempts limit (instance override or default)
	attemptsLimit := sm.ArchiveAttemptsMax
	if attemptsLimit <= 0 {
		attemptsLimit = 1000
	}

	success := false
	for counter < attemptsLimit {
		var err error
		archivePath, err = storage.ArchiveSessionFilePath(sm.sessionID, ts, counter)
		if err != nil {
			return "", fmt.Errorf("failed to determine archive path: %w", err)
		}

		err = sm.backend.ArchiveSession(sm.sessionID, archivePath)
		if err == nil {
			// success
			success = true
			break
		}
		if errors.Is(err, os.ErrExist) {
			// collision: try next counter
			counter++
			continue
		}
		return "", fmt.Errorf("failed to archive session: %w", err)
	}

	// If we never managed to archive the session (e.g. candidates all existed),
	// abort and return a clear error rather than clearing the session state.
	if !success {
		return "", fmt.Errorf("failed to archive session after %d attempts", attemptsLimit)
	}

	// Step 3: Clear state and reinitialize session
	sm.session.ScriptState = make(map[string]map[string]interface{})
	sm.session.SharedState = make(map[string]interface{})
	sm.session.CreatedAt = time.Now()
	sm.session.UpdatedAt = time.Now()
	sm.session.History = []storage.HistoryEntry{}

	// Reset history ring buffer
	sm.historyBuf = make([]storage.HistoryEntry, maxHistoryEntries)
	sm.historyStart = 0
	sm.historyLen = 0

	// Step 4: Persist the new empty session
	if err := sm.persistSessionInternal(); err != nil {
		return "", fmt.Errorf("failed to persist reset session: %w", err)
	}

	return archivePath, nil
}

// Close releases resources held by the state manager, persisting
// the session one final time before closing the backend.
// This method is atomic and idempotent.
func (sm *StateManager) Close() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	var persistErr error
	var closeErr error

	// 1. Persist the session before closing
	if sm.backend != nil {
		if err := sm.persistSessionInternal(); err != nil {
			log.Printf("WARNING: Failed to persist session during close: %v", err)
			persistErr = err // Record error but continue
		}
	}

	// 2. Close the backend
	if sm.backend != nil {
		closeErr = sm.backend.Close()
		sm.backend = nil // Set to nil *inside the lock* to prevent double-close
	}

	// 3. Return the first error encountered
	if persistErr != nil {
		return persistErr
	}
	return closeErr
}
