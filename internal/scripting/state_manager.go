package scripting

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/joeycumines/one-shot-man/internal/scripting/storage"
)

const sharedStateKey = "__shared__"
const maxHistoryEntries = 200

// StateManager orchestrates all persistence logic for the TUI.
type StateManager struct {
	mu        sync.Mutex // Protects session, registeredContracts, and history buffer
	backend   storage.StorageBackend
	sessionID string
	session   *storage.Session

	// Track registered contracts for validation
	registeredContracts map[string]storage.ContractDefinition

	// History ring buffer
	historyBuf   []storage.HistoryEntry // Fixed-size physical buffer (size == maxHistoryEntries)
	historyStart int                    // Index of the oldest entry in historyBuf
	historyLen   int                    // Number of valid, populated entries in historyBuf
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
	if session == nil {
		session = &storage.Session{
			Version:     "1.0.0",
			SessionID:   sessionID,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
			Contracts:   []storage.ContractDefinition{},
			History:     []storage.HistoryEntry{},
			LatestState: make(map[string]storage.ModeState),
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
				Contracts:   []storage.ContractDefinition{},
				History:     []storage.HistoryEntry{},
				LatestState: make(map[string]storage.ModeState),
			}
		}
	}

	sm := &StateManager{
		backend:             backend,
		sessionID:           sessionID,
		session:             session,
		registeredContracts: make(map[string]storage.ContractDefinition),
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

	if len(loadedHistory) > 0 {
		// Calculate truncation
		startIndex := 0
		if len(loadedHistory) > maxHistoryEntries {
			startIndex = len(loadedHistory) - maxHistoryEntries
		}

		// Copy the truncated data into our new buffer
		n := copy(sm.historyBuf, loadedHistory[startIndex:])

		// Set the logical length
		sm.historyLen = n
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

// RegisterContract registers a state contract for a mode.
// This must be called before attempting to restore state for that mode.
func (sm *StateManager) RegisterContract(contract storage.ContractDefinition) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Compute the contract hash
	hash, err := storage.ComputeContractSchemaHash(contract)
	if err != nil {
		return fmt.Errorf("failed to compute contract hash: %w", err)
	}

	// Determine the key (mode name or shared state key)
	key := contract.ModeName
	if contract.IsShared {
		key = sharedStateKey
	}

	// Store the contract
	sm.registeredContracts[key] = contract

	// Update the session's contract list
	// First, remove any existing contract with the same key
	newContracts := make([]storage.ContractDefinition, 0, len(sm.session.Contracts)+1)
	for _, c := range sm.session.Contracts {
		cKey := c.ModeName
		if c.IsShared {
			cKey = sharedStateKey
		}
		if cKey != key {
			newContracts = append(newContracts, c)
		}
	}
	newContracts = append(newContracts, contract)
	sm.session.Contracts = newContracts

	log.Printf("Registered contract for %q with hash %s", key, hash)

	return nil
}

// RestoreState attempts to restore persisted state for a mode.
// Returns the state as a JSON string if valid state is found, or an empty string otherwise.
// If the contract hash doesn't match, a warning is logged and no state is restored.
func (sm *StateManager) RestoreState(modeName string, isShared bool) (string, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	key := modeName
	if isShared {
		key = sharedStateKey
	}

	// Check if we have a registered contract for this mode
	contract, ok := sm.registeredContracts[key]
	if !ok {
		return "", fmt.Errorf("no contract registered for %q", key)
	}

	// Compute the current contract hash
	currentHash, err := storage.ComputeContractSchemaHash(contract)
	if err != nil {
		return "", fmt.Errorf("failed to compute contract hash: %w", err)
	}

	// Look up the persisted state
	modeState, ok := sm.session.LatestState[key]
	if !ok {
		// No persisted state found
		return "", nil
	}

	// Validate the contract hash
	if modeState.ContractHash != currentHash {
		log.Printf("WARNING: Contract hash mismatch for %q. Expected %s, got %s. Starting with fresh state.", key, currentHash, modeState.ContractHash)
		return "", nil
	}

	// Return the persisted state
	return modeState.StateJSON, nil
}

// CaptureSnapshot captures the current state of all modes into the session history.
// This creates an immutable history entry with the complete application state.
func (sm *StateManager) CaptureSnapshot(modeID, command string, stateMap map[string]string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Generate a unique entry ID
	entryID := fmt.Sprintf("%d", time.Now().UnixNano())

	// Compute contract hashes for all registered contracts
	contractHashes := make(map[string]string)
	for key, contract := range sm.registeredContracts {
		hash, err := storage.ComputeContractSchemaHash(contract)
		if err != nil {
			return fmt.Errorf("failed to compute contract hash for %q: %w", key, err)
		}
		contractHashes[key] = hash
	}

	// Serialize the complete state map
	stateJSON, err := json.Marshal(stateMap)
	if err != nil {
		return fmt.Errorf("failed to marshal state map: %w", err)
	}

	// Create the history entry
	entry := storage.HistoryEntry{
		EntryID:        entryID,
		ModeID:         modeID,
		Command:        command,
		Timestamp:      time.Now(),
		FinalState:     string(stateJSON),
		ContractHashes: contractHashes,
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

	// Update the latest state for each mode in the state map
	for key, stateJSON := range stateMap {
		// Find the contract for this key
		_, ok := sm.registeredContracts[key]
		if !ok {
			log.Printf("WARNING: No contract found for %q, skipping latest state update", key)
			continue
		}

		// Reuse the hash computed earlier instead of recomputing
		hash, ok := contractHashes[key]
		if !ok {
			log.Printf("WARNING: No contract hash found for %q, skipping latest state update", key)
			continue
		}

		// Update the latest state
		sm.session.LatestState[key] = storage.ModeState{
			ModeName:     key,
			ContractHash: hash,
			StateJSON:    stateJSON,
		}
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
